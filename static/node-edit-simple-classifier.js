Vue.component('node-edit-simple-classifier', {
	data: function() {
		return {
			configs: [],

			series: [],
			trainForm: {
				series: '',
				numClasses: '2',
				width: '0',
				height: '0',
			},
		};
	},
	props: ['initNode'],
	created: function() {
		try {
			var s = JSON.parse(this.initNode.Code);
			this.configs = s;
		} catch(e) {
			this.configs = [];
			this.addConfig();
		}
		myCall('GET', '/series', null, (data) => {
			this.series = [];
			data.forEach((el) => {
				if(!el.SrcVectorStr || el.DataType != 'int') {
					return;
				}
				this.series.push(el);
			});
		});
	},
	methods: {
		save: function() {
			let configs = [];
			this.configs.forEach((cfg) => {
				configs.push({
					ModelPath: cfg.ModelPath,
					NumClasses: parseInt(cfg.NumClasses),
					InputSize: [parseInt(cfg.InputSize[0]), parseInt(cfg.InputSize[1])],
				});
			});
			let code = JSON.stringify(configs);
			myCall('POST', '/queries/node?id='+this.initNode.ID, {
				code: code,
			});
		},
		addConfig: function() {
			this.configs.push({
				ModelPath: '',
				NumClasses: '2',
				InputSize: ['0', '0'],
			});
		},
		removeConfig: function(i) {
			this.configs.splice(i, 1);
		},
		train: function() {
			var params = {
				node_id: this.initNode.ID,
				series_id: this.trainForm.series,
				num_classes: this.trainForm.numClasses,
				width: this.trainForm.width,
				height: this.trainForm.height,
			}
			myCall('POST', '/simple-classifier/train', params);
		},
	},
	template: `
<div class="small-container m-2">
	<p>
		This is a simple image classification model that produces an integer class for each input video frame.
		If training a model, don't worry about the parameters below -- they will be automatically set after training completes.
	</p>
	<div v-for="(cfg, i) in configs">
		<h3>
			Config {{ i }}
			<button type="button" class="btn btn-danger btn-sm" v-on:click="removeConfig(i)">Remove</button>
		</h3>
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Model Path</label>
			<div class="col-sm-7">
				<input v-model="cfg.ModelPath" type="text" class="form-control">
				<small class="form-text text-muted">
					Path on the server's local disk to the model H5 file.
				</small>
			</div>
		</div>
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Number of Classes</label>
			<div class="col-sm-7">
				<input v-model="cfg.NumClasses" type="text" class="form-control">
			</div>
		</div>
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Input Width</label>
			<div class="col-sm-7">
				<input v-model="cfg.InputSize[0]" type="text" class="form-control">
			</div>
		</div>
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Input Height</label>
			<div class="col-sm-7">
				<input v-model="cfg.InputSize[1]" type="text" class="form-control">
			</div>
		</div>
	</div>
	<button v-on:click="addConfig" type="button" class="btn btn-primary">Add Config</button>
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
	<form v-on:submit.prevent="train">
		<h3>Training</h3>
		<p>To train a model, select an integer label series below. After pressing Train, monitor progress from Jobs.</p>
		<div class="form-group row">
			<label class="col-sm-4 col-form-label">Train on:</label>
			<div class="col-sm-8">
				<select v-model="trainForm.series" class="form-control">
					<option v-for="s in series" :value="s.ID">{{ s.Name }}</option>
				</select>
			</div>
		</div>
		<div class="form-group row">
			<label class="col-sm-4 col-form-label">Number of Classes</label>
			<div class="col-sm-8">
				<input type="text" class="form-control" v-model="trainForm.numClasses" />
				<small class="form-text text-muted">
					The number of distinct classes in your dataset.
				</small>
			</div>
		</div>
		<div class="form-group row">
			<label class="col-sm-4 col-form-label">Width</label>
			<div class="col-sm-8">
				<input type="text" class="form-control" v-model="trainForm.width" />
			</div>
		</div>
		<div class="form-group row">
			<label class="col-sm-4 col-form-label">Height</label>
			<div class="col-sm-8">
				<input type="text" class="form-control" v-model="trainForm.height" />
				<small class="form-text text-muted">
					The resolution at which the model should input video frames.
				</small>
			</div>
		</div>
		<button type="submit" class="btn btn-primary mx-2">Train</button>
	</form>
</div>
	`,
});
