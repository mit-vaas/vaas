Vue.component('explore-detail-detection', {
	data: function() {
		return {
			index: 0,
			labels: [],

			// index of detection in labels[index] that was selected
			selection: null,
		};
	},
	props: ['result'],
	created: function() {
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
				if(i == this.selection) {
					cfg.stroke = 'orange';
					cfg.strokeWidth = 5;
				}
				var rect = new Konva.Rect(cfg);
				rect.myidx = i;
				layer.add(rect);
			});
			layer.draw();
			layer.on('mouseover', function(e) {
				document.body.style.cursor = 'pointer';
				var shape = e.target;
				if(shape.myidx != this.selection) {
					shape.stroke('yellow');
					layer.draw();
				}
			});
			layer.on('mouseout', function(e) {
				document.body.style.cursor = 'default';
				var shape = e.target;
				if(shape.myidx != this.selection) {
					shape.stroke('red');
					layer.draw();
				}
			});
			layer.on('click', function(e) {
				var shape = e.target;
				if(this.selection == shape.myidx) {
					this.selection = null;
					shape.stroke('red');
				} else {
					stage.find('Rect').each(function(other) {
						other.stroke('red');
					});
					this.selection = shape.myidx;
					shape.stroke('orange');
				}
				layer.draw();
			});
		},
		next: function(amount) {
			this.index += amount;
			if(this.index < 0) {
				this.index = 0;
			} else if(this.index >= this.count) {
				this.index = this.count-1;
			}
			this.selection = null;
			Vue.nextTick(this.render);
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
</div>
	`,
});
