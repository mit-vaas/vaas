Vue.component('node-edit-rescale-resample', {
	data: function() {
		return {
			width: '0',
			height: '0',
			freq: '1',

			inputSeries: '',
			metricSeries: '',
			metricNode: '',

			tuneResults: null,
		};
	},
	props: ['initNode'],
	created: function() {
		try {
			var s = JSON.parse(this.initNode.Code);
			this.width = s.Width;
			this.height = s.Height;
			this.freq = s.Freq;
		} catch(e) {}
	},
	methods: {
		save: function() {
			var code = JSON.stringify({
				Width: parseInt(this.width),
				Height: parseInt(this.height),
				Freq: parseInt(this.freq),
			});
			$.post('/queries/node?id='+this.initNode.ID, {
				code: code,
			});
		},
	},
	template: `
<div class="small-container m-2">
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Output Width</label>
		<div class="col-sm-7">
			<input v-model="width" type="text" class="form-control">
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Output Height</label>
		<div class="col-sm-7">
			<input v-model="height" type="text" class="form-control">
		</div>
	</div>
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Re-sample Rate</label>
			<div class="col-sm-7">
				<input v-model="freq" type="text" class="form-control">
				<small id="emailHelp" class="form-text text-muted">
					This rate is measured relative to the query input rate (not the parent).
					For example, "4" would downsample 4x from the raw data capture rate.
				</small>
			</div>
		</div>
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
</div>
	`,
});
