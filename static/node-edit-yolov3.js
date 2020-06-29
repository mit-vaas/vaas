Vue.component('node-edit-yolov3', {
	data: function() {
		return {
			canvasSize: ['0', '0'],
			inputSize: ['0', '0'],
		};
	},
	props: ['initNode'],
	created: function() {
		try {
			var s = JSON.parse(this.initNode.Code);
			this.canvasSize = s.CanvasSize;
			this.inputSize = s.InputSize;
		} catch(e) {}
	},
	methods: {
		save: function() {
			var code = JSON.stringify({
				CanvasSize: [parseInt(this.canvasSize[0]), parseInt(this.canvasSize[1])],
				InputSize: [parseInt(this.inputSize[0]), parseInt(this.inputSize[1])],
			});
			$.post('/queries/node?id='+this.initNode.ID, {
				code: code,
			});
		},
	},
	template: `
<div class="small-container m-2">
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Canvas Width</label>
		<div class="col-sm-7">
			<input v-model="canvasSize[0]" type="text" class="form-control">
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Canvas Height</label>
		<div class="col-sm-7">
			<input v-model="canvasSize[1]" type="text" class="form-control">
			<small class="form-text text-muted">
				Rescale output detections from coordinates based on the parent video dimensions
				to a canvas of this size.
			</small>
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Input Width</label>
		<div class="col-sm-7">
			<input v-model="inputSize[0]" type="text" class="form-control">
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Input Height</label>
		<div class="col-sm-7">
			<input v-model="inputSize[1]" type="text" class="form-control">
			<small class="form-text text-muted">
				Rescale the input to this size before applying YOLOv3.
				Does not affect the output coordinate system.
			</small>
		</div>
	</div>
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
</div>
	`,
});
