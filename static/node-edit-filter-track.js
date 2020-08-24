Vue.component('node-edit-filter-track', {
	data: function() {
		return {
			canvasDims: [0, 0],
			orderMatters: 'yes',
			shapes: [[]],
			dataSeries: [],
			selectedSeries: null,
			curDrawing: null,
		};
	},
	props: ['initNode'],
	created: function() {
		try {
			var s = JSON.parse(this.initNode.Code);
			this.canvasDims = s.CanvasDims;
			this.orderMatters = (s.Order) ? 'yes' : 'no';
			this.shapes = s.Shapes;
		} catch(e) {}
		myCall('GET', '/datasets', null, (dataSeries) => {
			this.dataSeries = dataSeries;
		});
	},
	methods: {
		addAlt: function() {
			this.shapes.push([]);
			this.curDrawing = null;
		},
		removeAlt: function(i) {
			this.shapes.splice(i, 1);
			this.curDrawing = null;
		},
		addShape: function(i) {
			this.curDrawing = i;
		},
		removeShape: function(i, j) {
			this.shapes[i].splice(j, 1);
			this.curDrawing = null;
		},
		onDraw: function(e) {
			var shp;
			if(e.type == 'box') {
				shp = [[e.left, e.top], [e.right, e.bottom]];
			} else {
				shp = e.points;
			}
			this.canvasDims = e.dims;
			this.shapes[this.curDrawing].push(shp);
			this.curDrawing = null;
		},
		save: function() {
			var code = JSON.stringify({
				CanvasDims: [parseFloat(this.canvasDims[0]), parseFloat(this.canvasDims[1])],
				Shapes: this.shapes,
				Order: this.orderMatters == 'yes',
			});
			myCall('POST', '/queries/node?id='+this.initNode.ID, {
				code: code,
			});
		},
	},
	template: `
<div class="m-2 container">
	<p>Configure the track filter below. You can define one or more lists of shapes that the track must pass through. Each list is a different alternative, and tracks pass the filter if they match any alternative.</p>
	<div class="form-group row">
		<label class="col-sm-4 col-form-label">Matching Mode</label>
		<div class="col-sm-8">
			<div class="form-check">
				<input class="form-check-input" type="radio" v-model="orderMatters" value="yes">
				<label class="form-check-label">Sequence (order matters)</label>
			</div>
			<div class="form-check">
				<input class="form-check-input" type="radio" v-model="orderMatters" value="no">
				<label class="form-check-label">Set (any order)</label>
			</div>
			<small id="emailHelp" class="form-text text-muted">
				In sequence matching mode, for each alternative shape list, a track matches the alternative only if it passes through shapes in the list in order.
				<br />In set matching mode, a track matches the alternative if it passes through all of the shapes, in any order.
			</small>
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-2 col-form-label">Canvas Width</label>
		<div class="col-sm-10">
			<input v-model="canvasDims[0]" type="text" class="form-control">
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-2 col-form-label">Canvas Height</label>
		<div class="col-sm-10">
			<input v-model="canvasDims[1]" type="text" class="form-control">
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-4 col-form-label">Dataset</label>
		<div class="col-sm-8">
			<select v-model="selectedSeries" class="form-control mx-2">
				<option v-for="ds in dataSeries" :value="ds.ID">{{ ds.Name }}</option>
			</select>
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-4 col-form-label">Alternatives</label>
		<div class="col-sm-8">
			<template v-for="(alt, i) in shapes">
				<p>
					Alternative {{ i+1 }}
					<button type="button" class="btn btn-danger" v-on:click="removeAlt(i)">Remove</button>
				</p>
				<table class="table table-sm table-borderless">
					<tbody>
						<tr v-for="(shp, j) in alt">
							<td>{{ shp }}</td>
							<td><button type="button" class="btn btn-danger btn-sm" v-on:click="removeShape(i, j)">Remove</button></td>
						</tr>
						<tr>
							<td></td>
							<td>
								<button type="button" class="btn btn-primary btn-sm mx-2" v-on:click="addShape(i)" :disabled="selectedSeries == null">Add Shape</button>
							</td>
						</tr>
					</tbody>
				</table>
				<div
					v-if="curDrawing != null && curDrawing == i && selectedSeries != null"
					class="bordered-div p-2 m-2"
					>
					<util-video-draw-shape
						v-bind:series_id="selectedSeries"
						fixedOptions="box,polygon"
						v-on:draw="onDraw($event)">
					</util-video-draw-shape>
				</div>
			</template>
			<p><button type="button" class="btn btn-primary" v-on:click="addAlt">Add Alternative</button></p>
		</div>
	</div>
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
</div>
	`,
});
