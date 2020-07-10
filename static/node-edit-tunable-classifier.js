Vue.component('node-edit-tunable-classifier', {
	data: function() {
		return {
			modelPath: '',
			maxWidth: '',
			maxHeight: '',
			numClasses: '',
			scaleCount: '',
			depth: '',

			series: [],
			selectedSeries: '',
		};
	},
	props: ['initNode'],
	created: function() {
		try {
			var s = JSON.parse(this.initNode.Code);
			this.modelPath = s.ModelPath;
			this.maxWidth = s.MaxWidth;
			this.maxHeight = s.MaxHeight;
			this.numClasses = s.NumClasses;
			this.scaleCount = s.ScaleCount;
			this.depth = s.Depth;
		} catch(e) {}
		$.get('/series', function(data) {
			this.series = [];
			data.forEach((el) => {
				if(!el.SrcVectorStr) {
					return;
				}
				this.series.push(el);
			});
		}.bind(this));
	},
	methods: {
		save: function() {
			var code = JSON.stringify({
				ModelPath: this.modelPath,
				MaxWidth: parseInt(this.maxWidth),
				MaxHeight: parseInt(this.maxHeight),
				NumClasses: parseInt(this.numClasses),
				ScaleCount: parseInt(this.scaleCount),
				Depth: parseInt(this.depth),
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
			$.post('/tunable-classifier/train', params);
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
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Max Width</label>
		<div class="col-sm-7">
			<input v-model="maxWidth" type="text" class="form-control">
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Max Height</label>
		<div class="col-sm-7">
			<input v-model="maxHeight" type="text" class="form-control">
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Num Classes</label>
		<div class="col-sm-7">
			<input v-model="numClasses" type="text" class="form-control">
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Scale Count</label>
		<div class="col-sm-7">
			<input v-model="scaleCount" type="text" class="form-control">
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Depth</label>
		<div class="col-sm-7">
			<input v-model="depth" type="text" class="form-control">
		</div>
	</div>
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
	<form v-on:submit.prevent="train" class="form-inline my-2">
		<label>Train on:</label>
		<select v-model="selectedSeries" class="form-control mx-2">
			<option v-for="s in series" :value="s.ID">{{ s.Name }}</option>
		</select>
		<button type="submit" class="btn btn-primary mx-2">Train</button>
	</form>
</div>
	`,
});
