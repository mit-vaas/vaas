Vue.component('annotate-default-detection', {
	data: function() {
		return {
			response: null,
			imMeta: null,
			context1: null,
			context2: null,
			labels: [{'Detections': []}],
			working: [],
			mode: 'box',
			state: 'idle',

			// cache of unlabeled responses to use as examples
			nextCache: [],
		};
	},
	props: ['series'],
	created: function() {
		app.$on('keypress', (e) => {
			if(e.key == 'x') {
				this.cancelWorking();
			}
		});
		myCall('GET', '/series/labels?id='+this.series.ID+'&index=-1', null, this.updateImage);
	},
	methods: {
		cancelWorking: function() {
			this.context2.clearRect(0, 0, this.$refs.layer2.width, this.$refs.layer2.height);
			this.state = 'idle';
			this.working = [];
		},
		render: function() {
			this.context1.clearRect(0, 0, this.$refs.layer1.width, this.$refs.layer1.height);
			if(this.mode == 'point') {
				this.labels[0].Detections.forEach((el) => {
					this.context1.beginPath();
					this.context1.arc(el.left, el.top, 3, 0, 2*Math.PI,);
					this.context1.fillStyle = '#ff0000';
					this.context1.fill();
					this.context1.closePath();
				});
			} else if(this.mode == 'line') {
				this.labels[0].Detections.forEach((el) => {
					this.context1.beginPath();
					this.context1.moveTo(el.left, el.top);
					this.context1.lineTo(el.right, el.bottom);
					this.context1.lineWidth = 3;
					this.context1.strokeStyle = '#ff0000';
					this.context1.stroke();
					this.context1.closePath();
				});
			} else if(this.mode == 'box') {
				this.labels[0].Detections.forEach((el) => {
					this.context1.beginPath();
					this.context1.moveTo(el.left, el.top);
					this.context1.lineTo(el.left, el.bottom);
					this.context1.lineTo(el.right, el.bottom);
					this.context1.lineTo(el.right, el.top);
					this.context1.lineTo(el.left, el.top);
					this.context1.lineWidth = 3;
					this.context1.strokeStyle = '#ff0000';
					this.context1.stroke();
					this.context1.closePath();
				});
			}
		},
		updateImage: function(response) {
			this.response = response;
			if(response.Labels) {
				this.labels = response.Labels;
			} else {
				this.labels = [{'Detections': []}];
			}
			this.working = [];
			this.state = 'idle';

			myCall('GET', this.response.URLs[0]+'&type=meta', null, (meta) => {
				this.imMeta = meta;
				Vue.nextTick(() => {
					this.context1 = this.$refs.layer1.getContext('2d');
					this.context2 = this.$refs.layer2.getContext('2d');
					this.render();
				});
			});
		},
		getLabelsURL: function(index) {
			return '/series/labels?id='+this.series.ID+'&nframes=1&index='+index;
		},
		get: function(i) {
			if(i >= 0) {
				myCall('GET', this.getLabelsURL(i), null, this.updateImage);
				return;
			}
			var cacheResponse = () => {
				myCall('GET', this.getLabelsURL(-1), null, (response) => {
					this.nextCache.push(response);
				});
			};
			if(this.nextCache.length > 0) {
				cacheResponse();
				var response = this.nextCache.splice(0, 1)[0];
				this.updateImage(response);
				return
			}
			myCall('GET', this.getLabelsURL(-1), null, (response) => {
				this.updateImage(response);
				for(var j = 0; j < 8; j++) {
					cacheResponse();
				}
			});
		},
		click: function(e) {
			var rect = e.target.getBoundingClientRect();
			var x = e.clientX - rect.left;
			var y = e.clientY - rect.top;

			this.context2.clearRect(0, 0, this.$refs.layer2.width, this.$refs.layer2.height);
			if(e.which == 3) {
				if(this.state == 'idle') {
					return;
				}
				e.preventDefault();
				this.cancelWorking();
			} else if(this.state == 'idle' && this.mode != 'point') {
				this.state = this.mode;
				this.working.push([x, y]);
			} else if(this.mode == 'point' || this.state == 'line' || this.state == 'box') {
				var detection = {
					track_id: -1,
					left: x,
					top: y,
					right: x,
					bottom: y,
				};
				if(this.state == 'line' || this.state == 'box') {
					detection.left = Math.min(detection.left, this.working[0][0]);
					detection.top = Math.min(detection.top, this.working[0][1]);
					detection.right = Math.max(detection.right, this.working[0][0]);
					detection.bottom = Math.max(detection.bottom, this.working[0][1]);
				}
				this.labels[0].Detections.push(detection);
				this.cancelWorking();
				this.render();
			}
		},
		mousemove: function(e) {
			var rect = e.target.getBoundingClientRect();
			var x = e.clientX - rect.left;
			var y = e.clientY - rect.top;

			this.context2.clearRect(0, 0, this.$refs.layer2.width, this.$refs.layer2.height);
			if(this.state == 'line') {
				this.context2.beginPath();
				this.context2.moveTo(this.working[0][0], this.working[0][1]);
				this.context2.lineTo(x, y);
				this.context2.lineWidth = 3;
				this.context2.strokeStyle = '#ff0000';
				this.context2.stroke();
				this.context2.closePath();
			} else if(this.state == 'box') {
				this.context2.beginPath();
				this.context2.moveTo(this.working[0][0], this.working[0][1]);
				this.context2.lineTo(this.working[0][0], y);
				this.context2.lineTo(x, y);
				this.context2.lineTo(x, this.working[0][1]);
				this.context2.lineTo(this.working[0][0], this.working[0][1]);
				this.context2.lineWidth = 3;
				this.context2.strokeStyle = '#ff0000';
				this.context2.stroke();
				this.context2.closePath();
			}
		},
		prev: function() {
			if(this.response.Index < 0) {
				this.get(0);
			} else {
				var i = this.response.Index - 1;
				this.get(i);
			}
		},
		next: function() {
			if(this.response.Index < 0) {
				this.get(-1);
			} else {
				var i = this.response.Index+1;
				this.get(i);
			}
		},
		done: function() {
			this.labels[0].CanvasDims = [this.imMeta.Width, this.imMeta.Height];
			var params = {
				id: this.series.ID,
				index: this.response.Index,
				slice: this.response.Slice,
				labels: this.labels,
			};
			myCall('POST', '/series/detection-label', JSON.stringify(params), () => {
				if(this.response.Index < 0) {
					this.get(-1);
				} else {
					var i = this.response.Index+1;
					this.get(i);
				}
			});
		},
		clear: function() {
			this.labels = [[]];
			this.render();
		},
	},
	watch: {
		mode: function() {
			this.render();
		},
	},
	template: `
<div>
	<div>
		The tool will sample video frames from the configured source vector.
		Click on the image to draw points, lines, or boxes.
		After labeling objects in the frame, press Done to save the labels.
		Press Prev to view and modify previous annotations.
	</div>
	<div v-on:click="click($event)" v-on:mousemove="mousemove($event)" class="canvas-container">
		<template v-if="imMeta != null">
			<div :style="{
					width: imMeta.Width + 'px',
					height: imMeta.Height + 'px',
				}"
				>
				<img :src="response.URLs[0] + '&type=jpeg'" />
				<canvas :width="imMeta.Width" :height="imMeta.Height" ref="layer1"></canvas>
				<canvas :width="imMeta.Width" :height="imMeta.Height" ref="layer2"></canvas>
			</div>
		</template>
	</div>
	<div class="form-row align-items-center">
		<div class="col-auto">
			<select v-model="mode" class="form-control">
				<option value="point">Point</option>
				<option value="line">Line</option>
				<option value="box">Box</option>
			</select>
		</div>
		<div class="col-auto">
			<button v-on:click="prev" type="button" class="btn btn-primary">Prev</button>
		</div>
		<div class="col-auto">
			<template v-if="response != null">
				<span v-if="response.Index < 0">[New]</span>
				<span v-else>{{ response.Index }}</span>
			</template>
		</div>
		<div class="col-auto">
			<button v-on:click="next" type="button" class="btn btn-primary">Next</button>
		</div>
		<div class="col-auto">
			<button v-on:click="clear" type="button" class="btn btn-primary">Clear</button>
		</div>
		<div class="col-auto">
			<button v-on:click="done" type="button" class="btn btn-primary">Done</button>
		</div>
	</div>
</div>
	`,
});
