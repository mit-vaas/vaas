Vue.component('util-video-draw-shape', {
	data: function() {
		return {
			item: null,
			curDesc: '',
			mode: 'box',
			modeOptions: ['box', 'polyline', 'polygon'],
		};
	},
	props: ['series_id', 'fixedOptions'],
	created: function() {
		if(this.fixedOptions) {
			if(typeof(this.fixedOptions) == 'string') {
				this.modeOptions = this.fixedOptions.split(',');
			} else {
				this.modeOptions = this.fixedOptions;
			}
			this.mode = this.modeOptions[0];
		}
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
					await myCall('POST', '/series/random-slice', params, (data) => {
						slice = data;
					})
				})
				.then(async () => {
					var url = '/series/get-item?series_id='+this.series_id+'&segment_id='+slice.Segment.ID+'&start='+slice.Start+'&end='+slice.End;
					await myCall('GET', url+'&type=meta', null, (data) => {
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
				container: this.$refs.layer,
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
					var width = Math.abs(curRect.meta[0] - x);
					var height = Math.abs(curRect.meta[1] - y);
					curRect.x(Math.min(curRect.meta[0], x));
					curRect.y(Math.min(curRect.meta[1], y));
					curRect.width(width);
					curRect.height(height);
					var s = {
						type: 'box',
						left: parseInt(curRect.x()),
						top: parseInt(curRect.y()),
						right: parseInt(curRect.x() + curRect.width()),
						bottom: parseInt(curRect.y() + curRect.height()),
					};
					s.desc = `Box(${s.left}, ${s.top}, ${s.right}, ${s.bottom})`;
					this.curDesc = s.desc + ` (w=${width}, h=${height})`;
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
			} else if(this.mode == 'polygon') {
				var curLine = null;
				var points = [];
				var done = false;
				var finish = () => {
					var s = {
						type: 'polygon',
						points: [],
					};
					var descParts = [];
					points.forEach(function(point) {
						var x = parseInt(point.x());
						var y = parseInt(point.y());
						s.points.push([x, y]);
						descParts.push(`(${x}, ${y})`);
					});
					s.desc = 'Polygon(' + descParts.join(', ') + ')';
					this.curDesc = s.desc;
					this.$emit('draw', s);
				};
				var addPoint = (x, y) => {
					var point = new Konva.Circle({
						x: x,
						y: y,
						radius: 4,
						stroke: 'black',
						fill: 'yellow',
						strokeWidth: 2,
					});
					var idx = points.length;
					points.push(point);
					point.on('click', (e) => {
						e.cancleBubble = true;
						if(idx == 0 && points.length >= 3) {
							done = true;
							var prevPoints = curLine.points();
							prevPoints = prevPoints.slice(0, prevPoints.length - 2);
							curLine.points(prevPoints);
							curLine.fill('rgba(255, 0, 0, 0.5)');
							curLine.closed(true);
							finish();
							layer.draw();
							return;
						}
						points.slice(idx).forEach((point) => {
							point.destroy();
						});
						points = points.slice(0, idx)
						if(curLine != null) {
							if(idx == 0) {
								curLine.destroy();
							} else {
								curLine.points(curLine.points().slice(0, 2*(idx+1)));
							}
						}
						curLine.fill(null);
						curLine.closed(false);
						layer.draw();
						done = false;
					});
					point.on('mouseenter', () => {
						point.radius(6);
						point.strokeWidth(3);
						layer.draw();
					});
					point.on('mouseleave', () => {
						point.radius(4);
						point.strokeWidth(2);
						layer.draw();
					});
					layer.add(point);
				};
				stage.on('click', () => {
					if(done) {
						return;
					}
					var pos = stage.getPointerPosition();
					if(curLine == null) {
						curLine = new Konva.Line({
							points: [pos.x, pos.y, pos.x, pos.y],
							stroke: 'yellow',
							strokeWidth: 3,
						});
						layer.add(curLine);
						addPoint(pos.x, pos.y);
						layer.draw();
						return
					}
					var prevPoints = curLine.points();
					prevPoints = prevPoints.slice(0, prevPoints.length - 2);
					curLine.points(prevPoints.concat([pos.x, pos.y, pos.x, pos.y]));
					addPoint(pos.x, pos.y);
					layer.draw();
				})
				stage.on('mousemove', () => {
					if(curLine == null || done) {
						return;
					}
					var pos = stage.getPointerPosition();
					var prevPoints = curLine.points();
					prevPoints = prevPoints.slice(0, prevPoints.length - 2);
					curLine.points(prevPoints.concat([pos.x, pos.y]));
					layer.draw();
				});
			}
		},
	},
	template: `
<div>
	<form v-if="modeOptions.length > 1" class="form-inline">
		<label>Mode</label>
		<select v-model="mode" @change="render" class="form-control mx-2">
			<option v-for="opt in modeOptions" :value="opt">{{ opt | capitalize }}</option>
		</select>
	</form>
	<div
		v-if="item != null"
		class="canvas-container"
		:style="{
				width: item.Width + 'px',
				height: item.Height + 'px',
		}"
		>
		<img :src="item.imageURL" :width="item.Width" :height="item.Height" />
		<div class="konva" ref="layer"></div>
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
