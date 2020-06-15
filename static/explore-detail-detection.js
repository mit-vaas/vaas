Vue.component('explore-detail-detection', {
	data: function() {
		return {
			index: 0,
			labels: [],
			mode: '', // detection or track

			// detection: index of detection in labels[index] that was selected
			// track: track ID
			selectedID: null,

			selection: null,

			exporting: false,
			scatterURL: '',
		};
	},
	props: ['result'],
	created: function() {
		this.mode = this.result.Type;
		$.get(this.result.URL + '&type=labels', function(labels) {
			this.labels = labels;
			this.render();
		}.bind(this), 'json');
	},
	methods: {
		render: function() {
			var stage = new Konva.Stage({
				container: '#konva',
				width: this.result.Width,
				height: this.result.Height,
			});
			var layer = new Konva.Layer();
			stage.add(layer);
			this.labels[this.index].forEach(function(el, i) {
				var cfg = {
					x: el.left,
					y: el.top,
					width: el.right-el.left,
					height: el.bottom-el.top,
					stroke: 'red',
					strokeWidth: 3,
				};
				var myid;
				if(this.mode == 'detection') {
					myid = i;
				} else {
					myid = el.track_id;
				}
				if(myid == this.selectedID) {
					cfg.stroke = 'orange';
					cfg.strokeWidth = 5;
				}
				var rect = new Konva.Rect(cfg);
				rect.myid = myid;
				layer.add(rect);
			});
			layer.draw();
			layer.on('mouseover', function(e) {
				document.body.style.cursor = 'pointer';
				var shape = e.target;
				if(shape.myid != this.selectedID) {
					shape.stroke('yellow');
					layer.draw();
				}
			});
			layer.on('mouseout', function(e) {
				document.body.style.cursor = 'default';
				var shape = e.target;
				if(shape.myid != this.selectedID) {
					shape.stroke('red');
					layer.draw();
				}
			});
			layer.on('click', function(e) {
				var shape = e.target;
				if(this.selectedID == shape.myid) {
					this.selectedID = null;
					this.selection = null;
					shape.stroke('red');
				} else {
					stage.find('Rect').each(function(other) {
						other.stroke('red');
					});
					this.selectedID = shape.myid;
					shape.stroke('orange');
					this.updateSelection();
				}
				layer.draw();
			}.bind(this));
		},
		updateSelection: function() {
			if(this.mode == 'detection') {
				var origSlice = this.result.Slice;
				this.selection = [{
					Slice: {
						Start: origSlice.Start + this.index,
						End: origSlice.Start + this.index + 1,
						Clip: {ID: origSlice.Clip.ID},
					},
					Data: {
						Type: 'detection',
						Detections: [[this.labels[this.index][this.selectedID]]],
					},
				}];
			} else if(this.mode == 'track') {
				// collect detections for the segment of video where track is alive
				var trackID = this.selectedID;
				var detections = [];
				var firstFrame = null;
				this.labels.forEach(function(dlist, frameIdx) {
					if(!dlist) {
						return;
					}
					dlist.forEach(function(el) {
						if(el.track_id != trackID) {
							return;
						}
						if(firstFrame == null) {
							firstFrame = frameIdx;
						}
						while(detections.length <= (frameIdx-firstFrame)) {
							detections.push([]);
						}
						detections[frameIdx-firstFrame] = [el];
					});
				});
				var origSlice = this.result.Slice;
				this.selection = [{
					Slice: {
						Start: origSlice.Start + firstFrame,
						End: origSlice.Start + firstFrame + detections.length,
						Clip: {ID: origSlice.Clip.ID},
					},
					Data: {
						Type: 'track',
						Detections: detections,
					},
				}];

				$.ajax({
					type: "POST",
					url: '/aggregates/scatter',
					data: JSON.stringify(this.selection),
					success: function(data) {
						this.scatterURL = data.URL;
					}.bind(this),
				});
			}
		},
		next: function(amount) {
			this.index += amount;
			if(this.index < 0) {
				this.index = 0;
			} else if(this.index >= this.count) {
				this.index = this.count-1;
			}
			if(this.mode == 'detection') {
				this.selectedIdx = null;
				this.selection = null;
			}
			Vue.nextTick(this.render);
		},
		exportData: function() {

		},
	},
	computed: {
		imageURL: function() {
			var clipID = this.result.Slice.Clip.ID;
			var start = this.result.Slice.Start+this.index;
			var end = start+1;
			return '/clips/get?type=jpeg&id='+clipID+'&start='+start+'&end='+end;
		},
		count: function() {
			return this.result.Slice.End - this.result.Slice.Start;
		},
		selectionJSON: function() {
			return JSON.stringify(this.selection.Data.Detections);
		},
	},
	template: `
<div>
	<div class="canvas-container">
		<template v-if="imageURL != ''">
			<div :style="{
					width: result.Width + 'px',
					height: result.Height + 'px',
				}"
				>
				<img :src="imageURL" />
				<!--<canvas :width="result.Width" :height="result.Height" ref="layer"></canvas>-->
				<div id="konva" class="konva" ref="layer"></div>
			</div>
		</template>
	</div>
	<div class="form-row align-items-center">
		<div class="col-auto">
			<button v-on:click="next(-250)" type="button" class="btn btn-primary">&lt;&lt;&lt;</button>
		</div>
		<div class="col-auto">
			<button v-on:click="next(-25)" type="button" class="btn btn-primary">&lt;&lt;</button>
		</div>
		<div class="col-auto">
			<button v-on:click="next(-1)" type="button" class="btn btn-primary">&lt;</button>
		</div>
		<div class="col-auto">
			{{ index }}/{{ count }}
		</div>
		<div class="col-auto">
			<button v-on:click="next(1)" type="button" class="btn btn-primary">&gt;</button>
		</div>
		<div class="col-auto">
			<button v-on:click="next(25)" type="button" class="btn btn-primary">&gt;&gt;</button>
		</div>
		<div class="col-auto">
			<button v-on:click="next(250)" type="button" class="btn btn-primary">&gt;&gt;&gt;</button>
		</div>
	</div>
	<div v-if="selection != null">
		<template v-if="mode == 'detection'">
			<p>{{ selectionJSON }}</p>
			<button v-on:click="exportData" type="button" class="btn btn-primary">Export</button>
		</template>
		<template v-else-if="mode == 'track'">
			<div><img v-if="scatterURL != ''" :src="scatterURL" /></div>
			<button v-on:click="exportData" type="button" class="btn btn-primary">Export</button>
		</template>
		<export-modal v-if="exporting" v-bind:target="selection"></export-modal>
	</div>
</div>
	`,
});
