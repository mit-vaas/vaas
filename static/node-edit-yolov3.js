Vue.component('node-edit-yolov3', {
	data: function() {
		return {
			inputSize: ['0', '0'],
			configPath: '',
			modelPath: '',

			vectors: [],
			selectedVector: '',
		};
	},
	props: ['initNode'],
	created: function() {
		try {
			var s = JSON.parse(this.initNode.Code);
			this.inputSize = s.InputSize;
			this.configPath = s.ConfigPath;
			this.modelPath = s.ModelPath;
		} catch(e) {}
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
			var code = JSON.stringify({
				InputSize: [parseInt(this.inputSize[0]), parseInt(this.inputSize[1])],
				ConfigPath: this.configPath,
				ModelPath: this.modelPath,
			});
			myCall('POST', '/queries/node?id='+this.initNode.ID, {
				code: code,
			});
		},
		train: function() {
			var params = {
				node_id: this.initNode.ID,
				vector_id: this.selectedVector,
			}
			myCall('POST', '/yolov3/train', params);
		},
	},
	template: `
<div class="small-container m-2">
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
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Config Path</label>
		<div class="col-sm-7">
			<input v-model="configPath" type="text" class="form-control">
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Model Path</label>
		<div class="col-sm-7">
			<input v-model="modelPath" type="text" class="form-control">
		</div>
	</div>
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
	<form v-on:submit.prevent="train" class="form-inline my-2">
		<label>Train on:</label>
		<select v-model="selectedVector" class="form-control mx-2">
			<option v-for="vector in vectors" :key="vector.ID" :value="vector.ID">{{ vector.Vector | prettyVector }}</option>
		</select>
		<button type="submit" class="btn btn-primary mx-2">Train</button>
	</form>
</div>
	`,
});
