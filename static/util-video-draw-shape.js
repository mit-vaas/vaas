Vue.component('util-video-draw-shape', {
	data: function() {
		return {
			item: null,
			curDesc: '',
			mode: 'box',
		};
	},
	props: ['series_id'],
	created: function() {
		this.refresh();
	},
	methods: {
		refresh: function() {
			var slice;
			var item;
			Promise.resolve()
				.then(async () => {
					var params = {
						series_id: this.series_id,
						unit: 1,
					};
					await $.post('/series/random-slice', params, (data) => {
						slice = data;
					})
				})
				.then(async () => {
					var url = '/series/get-item?series_id='+this.series_id+'&segment_id='+slice.Segment.ID+'&start='+slice.Start+'&end='+slice.End;
					await $.get(url+'&type=meta', (data) => {
						item = data;
						item.imageURL = url+'&type=jpeg';
					});
				})
				.then(() => {
					var needRender = this.item == null || this.item.Width != item.Width || this.item.Height != item.Height;
					this.item = item;
					if(needRender) {
						Vue.nextTick(this.render);
					}
				});
		},
		render: function() {
			this.curDesc = '';
			var stage = new Konva.Stage({
				container: '#konva',
				width: this.item.Width,
				height: this.item.Height,
			});
			var layer = new Konva.Layer();
			stage.add(layer);
			layer.draw();

			if(this.mode == 'box') {
				var curRect = null;
				var done = false;
				var updateRect = (x, y) => {
					curRect.x(Math.min(curRect.meta[0], x));
					curRect.y(Math.min(curRect.meta[1], y));
					curRect.width(Math.abs(curRect.meta[0] - x));
					curRect.height(Math.abs(curRect.meta[1] - y));
					var s = {
						type: 'box',
						left: curRect.x(),
						top: curRect.y(),
						right: curRect.x() + curRect.width(),
						bottom: curRect.y() + curRect.height(),
					};
					this.curDesc = `Box(${s.left}, ${s.top}, ${s.right}, ${s.bottom})`;
					return s;
				};
				stage.on('click', () => {
					if(done) {
						return;
					}
					var pos = stage.getPointerPosition();
					if(curRect == null) {
						curRect = new Konva.Rect({
							x: pos.x,
							y: pos.y,
							width: 1,
							height: 1,
							stroke: 'yellow',
							strokeWidth: 3,
						});
						curRect.meta = [pos.x, pos.y];
						layer.add(curRect);
						layer.draw();
					} else {
						var s = updateRect(pos.x, pos.y);
						done = true;
						this.$emit('draw', s);
					}
				});
				stage.on('mousemove', () => {
					if(curRect == null || done) {
						return;
					}
					var pos = stage.getPointerPosition();
					updateRect(pos.x, pos.y);
					layer.draw();
				});
			}
		},
	},
	template: `
<div>
	<div
		v-if="item != null"
		class="canvas-container"
		:style="{
				width: item.Width + 'px',
				height: item.Height + 'px',
		}"
		>
		<img :src="item.imageURL" :width="item.Width" :height="item.Height" />
		<div id="konva" class="konva" ref="layer"></div>
	</div>
	<div>
		<p>{{ curDesc }}</p>
		<p>
			<button v-on:click="refresh" type="button" class="btn btn-primary">Re-sample</button>
			<button v-on:click="render" type="button" class="btn btn-primary">Clear</button>
		</p>
	</div>
</div>
	`,
});
