Vue.component('node-edit-yolov3', {
	data: function() {
		return {
			configs: [],

			vectors: [],
			trainForm: {
				vector: '',
				width: '',
				height: '',
				configPath: '',
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
		myCall('GET', '/vectors', null, (data) => {
			this.vectors = [];
			data.forEach((vector) => {
				if(vector.Vector.length != 2) {
					return;
				} else if(vector.Vector[0].DataType != 'video' || vector.Vector[1].DataType != 'detection') {
					return;
				}
				this.vectors.push(vector);
			});
		});
	},
	methods: {
		save: function() {
			let configs = [];
			this.configs.forEach((cfg) => {
				configs.push({
					InputSize: [parseInt(cfg.InputSize[0]), parseInt(cfg.InputSize[1])],
					ConfigPath: cfg.ConfigPath,
					ModelPath: cfg.ModelPath,
					MetaPath: cfg.MetaPath,
				});
			});
			let code = JSON.stringify(configs);
			myCall('POST', '/queries/node?id='+this.initNode.ID, {
				code: code,
			});
		},
		addConfig: function() {
			this.configs.push({
				InputSize: ['0', '0'],
				ConfigPath: '',
				ModelPath: '',
				MetaPath: '',
			});
		},
		removeConfig: function(i) {
			this.configs.splice(i, 1);
		},
		train: function() {
			var params = {
				node_id: this.initNode.ID,
				vector_id: this.trainForm.vector,
				width: this.trainForm.width,
				height: this.trainForm.height,
				config_path: this.trainForm.configPath,
			}
			myCall('POST', '/yolov3/train', params);
		},
	},
	template: `
<div class="small-container m-2">
	<div v-for="(cfg, i) in configs">
		<h3>
			Config {{ i }}
			<button type="button" class="btn btn-danger btn-sm" v-on:click="removeConfig(i)">Remove</button>
		</h3>
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
				<small class="form-text text-muted">
					Rescale the input to this size before applying YOLOv3.
					Does not affect the output coordinate system.
				</small>
			</div>
		</div>
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Config Path</label>
			<div class="col-sm-7">
				<input v-model="cfg.ConfigPath" type="text" class="form-control">
				<small class="form-text text-muted">
					If blank, defaults to YOLOv3 model trained on COCO.
				</small>
			</div>
		</div>
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Model Path</label>
			<div class="col-sm-7">
				<input v-model="cfg.ModelPath" type="text" class="form-control">
				<small class="form-text text-muted">
					If blank, defaults to YOLOv3 model trained on COCO.
				</small>
			</div>
		</div>
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Meta Path</label>
			<div class="col-sm-7">
				<input v-model="cfg.MetaPath" type="text" class="form-control">
				<small class="form-text text-muted">
					If blank, defaults to YOLOv3 model trained on COCO.
				</small>
			</div>
		</div>
	</div>
	<button v-on:click="addConfig" type="button" class="btn btn-primary">Add Config</button>
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
	<form v-on:submit.prevent="train">
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Train on:</label>
			<div class="col-sm-7">
				<select v-model="trainForm.vector" class="form-control">
					<option v-for="vector in vectors" :key="vector.ID" :value="vector.ID">{{ vector.Vector | prettyVector }}</option>
				</select>
			</div>
		</div>
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Input Width</label>
			<div class="col-sm-7">
				<input v-model="trainForm.width" type="text" class="form-control">
			</div>
		</div>
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Input Height</label>
			<div class="col-sm-7">
				<input v-model="trainForm.height" type="text" class="form-control">
			</div>
		</div>
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Config Path</label>
			<div class="col-sm-7">
				<input v-model="trainForm.configPath" type="text" class="form-control">
			</div>
		</div>
		<button type="submit" class="btn btn-primary mx-2">Train</button>
	</form>
</div>
	`,
});
