Vue.component('node-edit-rescale', {
	data: function() {
		return {
			width: '',
			height: '',
		};
	},
	props: ['initNode'],
	created: function() {
		try {
			var s = JSON.parse(this.initNode.Code);
			this.width = s.Width;
			this.height = s.Height;
		} catch(e) {}
	},
	methods: {
		save: function() {
			var code = JSON.stringify({
				Width: parseInt(this.width),
				Height: parseInt(this.height),
			});
			$.post('/queries/node?id='+this.initNode.ID, {
				code: code,
			});
		},
	},
	template: `
<div class="small-container m-2">
	<div class="form-group row">
		<label class="col-sm-2 col-form-label">Output Width</label>
		<div class="col-sm-10">
			<input v-model="width" type="text" class="form-control">
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-2 col-form-label">Output Height</label>
		<div class="col-sm-10">
			<input v-model="height" type="text" class="form-control">
		</div>
	</div>
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
</div>
	`,
});
