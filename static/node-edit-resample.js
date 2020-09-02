Vue.component('node-edit-resample', {
	data: function() {
		return {
			freq: '1',
		};
	},
	props: ['initNode'],
	created: function() {
		try {
			var s = JSON.parse(this.initNode.Code);
			this.freq = s.Freq;
		} catch(e) {}
	},
	methods: {
		save: function() {
			var code = JSON.stringify({
				Freq: parseInt(this.freq),
			});
			myCall('POST', '/queries/node?id='+this.initNode.ID, {
				code: code,
			});
		},
	},
	template: `
<div class="small-container m-2">
	<p>This node re-samples its input at a different framerate.</p>
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Re-sample Rate</label>
		<div class="col-sm-7">
			<input v-model="freq" type="text" class="form-control">
			<small class="form-text text-muted">
				This rate is measured relative to the query input rate (not the parent).
				For example, "4" would downsample 4x from the raw data capture rate.
			</small>
		</div>
	</div>
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
</div>
	`,
});
