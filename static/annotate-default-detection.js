Vue.component('annotate-default-detection', {
	data: function() {
		return {
			image: null,
			context1: null,
			context2: null,
			labels: [],
			working: [],
			mode: 'line',
			state: 'idle',
		};
	},
	props: ['ls'],
	created: function() {
		app.$on('keypress', function(e) {
			if(e.key == 'x') {
				this.cancelWorking();
			}
		}.bind(this));
		$.get('/labelsets/labels?id='+this.ls.ID+'&index=-1', this.updateImage, 'json');
	},
	methods: {
		cancelWorking: function() {
			this.context2.clearRect(0, 0, this.$refs.layer2.width, this.$refs.layer2.height);
			this.state = 'idle';
			this.working = [];
		},
		render: function() {
			this.context1.clearRect(0, 0, this.$refs.layer1.width, this.$refs.layer1.height);
			if(this.mode == 'line') {
				this.labels.forEach(function(el) {
					this.context1.beginPath();
					this.context1.moveTo(el[0][0], el[0][1]);
					this.context1.lineTo(el[1][0], el[1][1]);
					this.context1.lineWidth = 3;
					this.context1.strokeStyle = '#ff0000';
					this.context1.stroke();
					this.context1.closePath();
				}.bind(this));
			}
		},
		updateImage: function(image) {
			this.image = image;
			if(image.Labels) {
				this.labels = image.Labels;
			} else {
				this.labels = [];
			}
			this.working = [];
			this.state = 'idle';

			Vue.nextTick(function() {
				this.context1 = this.$refs.layer1.getContext('2d');
				this.context2 = this.$refs.layer2.getContext('2d');
				this.render();
			}.bind(this));
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
			} else if(this.state == 'idle') {
				if(this.mode == 'point') {
					// ...
				} else if(this.mode == 'line') {
					this.state = 'line';
					this.working.push([x, y]);
				}
			} else if(this.state == 'line') {
				var line = [[this.working[0][0], this.working[0][1]], [x, y]];
				this.labels.push(line);
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
			}
		},
		prev: function() {
			if(this.index < 0) {
				$.get('/labelsets/labels?id='+this.ls.ID+'&index=0', this.updateImage, 'json');
			} else {
				var i = this.index - 1;
				$.get('/labelsets/labels?id='+this.ls.ID+'&index='+i, this.updateImage, 'json');
			}
		},
		next: function() {
			if(this.index < 0) {
				$.get('/labelsets/labels?id='+this.ls.ID+'&index=-1', this.updateImage, 'json');
			} else {
				var i = this.index+1;
				$.get('/labelsets/labels?id='+this.ls.ID+'&index='+i, this.updateImage, 'json');
			}
		},
		done: function() {
			var params = {
				id: this.ls.ID,
				index: this.image.Index,
				uuid: this.image.UUID,
				labels: this.labels,
			};
			$.ajax({
				type: "POST",
				url: '/labelsets/detection-label',
				data: JSON.stringify(params),
				processData: false,
				success: function() {
					if(this.index < 0) {
						$.get('/labelsets/labels?id='+this.ls.ID+'&index=-1', this.updateImage, 'json');
					} else {
						var i = this.index+1;
						$.get('/labelsets/labels?id='+this.ls.ID+'&index='+i, this.updateImage, 'json');
					}
				}.bind(this),
			});
		},
		clear: function() {
			this.labels = [];
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
	<div v-on:click="click($event)" v-on:mousemove="mousemove($event)" id="a-d-container">
		<template v-if="image != null">
			<div :style="{
					width: image.Width + 'px',
					height: image.Height + 'px',
				}"
				>
				<img :src="image.URL" />
				<canvas :width="image.Width" :height="image.Height" ref="layer1"></canvas>
				<canvas :width="image.Width" :height="image.Height" ref="layer2"></canvas>
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
			<template v-if="image != null">
				<span v-if="image.Index < 0">[New]</span>
				<span v-else>{{ image.Index }}</span>
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
