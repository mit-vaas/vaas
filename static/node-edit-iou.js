Vue.component('node-edit-iou', {
	data: function() {
		return {
			maxAge: 10,
		};
	},
	props: ['initNode'],
	created: function() {
		try {
			var s = JSON.parse(this.initNode.Code);
			this.maxAge = s.maxAge;
		} catch(e) {}
	},
	methods: {
		save: function() {
			var code = JSON.stringify({
				maxAge: parseInt(this.maxAge),
			});
			myCall('POST', '/queries/node?id='+this.initNode.ID, {
				code: code,
			});
		},
	},
	template: `
<div class="small-container m-2">
	<div>
		<p>This node requires a detection parent, and produces tracks. Multi-object tracking is performed based on bounding box overlap: overlaping detections are linked together.</p>
	</div>
	<div class="form-group row">
		<label class="col-sm-2 col-form-label">Max Age</label>
		<div class="col-sm-10">
			<input v-model="maxAge" type="text" class="form-control">
			<small id="emailHelp" class="form-text text-muted">Number of frames until track is considered dead.</small>
		</div>
	</div>
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
</div>
	`,
});
