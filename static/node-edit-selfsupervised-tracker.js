Vue.component('node-edit-selfsupervised-tracker', {
	data: function() {
		return {
			modelPath: '',

			dataSeries: [],
			selectedSeries: '',
		};
	},
	props: ['initNode'],
	created: function() {
		try {
			var s = JSON.parse(this.initNode.Code);
			this.modelPath = s.ModelPath;
		} catch(e) {}
		$.get('/datasets', function(data) {
			this.dataSeries = data;
		}.bind(this));
	},
	methods: {
		save: function() {
			var code = JSON.stringify({
				ModelPath: this.modelPath,
			});
			$.post('/queries/node?id='+this.initNode.ID, {
				code: code,
			});
		},
		train: function() {
			var params = {
				node_id: this.initNode.ID,
				series_id: this.selectedSeries,
			}
			$.post('/selfsupervised-tracker/train', params);
		},
	},
	template: `
<div class="small-container m-2">
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Model Path</label>
		<div class="col-sm-7">
			<input v-model="modelPath" type="text" class="form-control">
		</div>
	</div>
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
	<form v-on:submit.prevent="train" class="form-inline my-2">
		<label>Train on:</label>
		<select v-model="selectedSeries" class="form-control mx-2">
			<option v-for="s in dataSeries" :value="s.ID">{{ s.Name }}</option>
		</select>
		<button type="submit" class="btn btn-primary mx-2">Train</button>
	</form>
</div>
	`,
});
