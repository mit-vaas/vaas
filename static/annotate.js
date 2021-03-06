Vue.component('annotate-tab', {
	data: function() {
		return {
			mode: 'list',
			labelSeries: [],

			vectors: [],
			newSetFields: {},

			availableTools: [
				'default-int',
				'default-detection',
			],

			annotateTool: '',
			visualizeTool: '',
			selectedSeries: null,
		};
	},
	props: ['tab'],
	created: function() {
		this.fetchLabelSeries(true);
		setInterval(this.fetchLabelSeries(), 5000);
	},
	methods: {
		fetchLabelSeries: function(force) {
			if(!force && this.tab != '#annotate-panel') {
				return;
			}
			myCall('GET', '/labelseries', null, (data) => {
				this.labelSeries = data;
			});
		},
		showNewLabelSeriesModal: function() {
			myCall('GET', '/vectors', null, (data) => {
				this.vectors = data;
				this.newSetFields = {
					name: '',
					type: 'detection',
					vector: '',
					tool: '',
				};
				$('#a-new-series-modal').modal('show');
			});
		},
		createSeries: function() {
			$('#a-new-series-modal').modal('hide');
			var params = {
				name: this.newSetFields.name,
				type: this.newSetFields.type,
				src: this.newSetFields.vector,
				metadata: JSON.stringify({'Tool': this.newSetFields.tool}),
			};
			myCall('POST', '/labelseries', params, (series) => {
				this.selectedSeries = series;
				var metadata = JSON.parse(series.AnnotateMetadata);
				this.annotateTool = 'annotate-' + metadata.Tool;
				this.mode = 'annotate';
			});
		},
		annotateLabels: function(series) {
			this.selectedSeries = series;
			this.annotateTool = 'annotate-default-' + series.DataType;
			this.mode = 'annotate';
		},
		visualizeLabels: function(series) {
			this.selectedSeries = series;
			this.visualizeTool = 'annotate-visualize';
			this.mode = 'visualize';
		},
		deleteSeries: function(series_id) {
			myCall('POST', '/series/delete', {'series_id': series_id}, () => {
				this.fetchLabelSeries(true);
			});
		},
	},
	watch: {
		tab: function() {
			if(this.tab != '#annotate-panel') {
				return;
			}
			this.mode = 'list';
			this.fetchLabelSeries(true);
		},
	},
	template: `
<div>
	<template v-if="mode == 'list'">
		<div class="my-1">
			<h3>Label Series</h3>
			<p>
				Label series associate hand-annotated data with each timestep (videoframe) of a dataset.
				After creating a label series and annotating several examples, a machine learning model such as YOLOv3 or simple classifier can be trained.
			</p>
		</div>
		<div class="my-1">
			<button v-on:click="showNewLabelSeriesModal" class="btn btn-primary">New Label Series</button>
		</div>
		<table class="table">
			<thead>
				<tr>
					<th>Name</th>
					<th>Source</th>
					<th>Type</th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				<tr v-for="series in labelSeries">
					<td>{{ series.Name }}</td>
					<td>{{ series.SrcVector | prettyVector }}</td>
					<td>{{ series.DataType }}</td>
					<td>
						<button v-on:click="annotateLabels(series)" class="btn btn-primary btn-sm">Annotate</button>
						<button v-on:click="visualizeLabels(series)" class="btn btn-primary btn-sm">Visualize</button>
						<button v-on:click="deleteSeries(series.ID)" class="btn btn-danger btn-sm">Delete</button>
					</td>
				</tr>
			</tbody>
		</table>
		<div class="modal" tabindex="-1" role="dialog" id="a-new-series-modal">
			<div class="modal-dialog" role="document">
				<div class="modal-content">
					<div class="modal-body">
						<form v-on:submit.prevent="createSeries">
							<div class="form-group row">
								<label class="col-sm-2 col-form-label">Name</label>
								<div class="col-sm-10">
									<input v-model="newSetFields.name" class="form-control" type="text" />
								</div>
							</div>
							<div class="form-group row">
								<label class="col-sm-2 col-form-label">Type</label>
								<div class="col-sm-10">
									<select v-model="newSetFields.type" class="form-control">
										<option value="detection">Detection</option>
										<option value="track">Track</option>
										<option value="int">Integer</option>
										<option value="video">Video</option>
									</select>
								</div>
							</div>
							<div class="form-group row">
								<label class="col-sm-2 col-form-label">Source</label>
								<div class="col-sm-10">
									<select v-model="newSetFields.vector" class="form-control">
										<option v-for="vector in vectors" :key="vector.ID" :value="vector.VectorStr">{{ vector.Vector | prettyVector }}</option>
									</select>
									<small class="form-text text-muted">
										The source specifies the data on which annotations should be made.
										The current annotation tools all use a source consisting of a single video series.
									</small>
								</div>
							</div>
							<div class="form-group row">
								<label class="col-sm-2 col-form-label">Annotation Tool</label>
								<div class="col-sm-10">
									<select v-model="newSetFields.tool" class="form-control">
										<option v-for="tool in availableTools" :key="tool" :value="tool">{{ tool }}</option>
									</select>
									<small class="form-text text-muted">
										Currently there are only two annotation tools.
										To annotate detections, use default-detection, and to annotate integers (e.g. classes), use default-int.
									</small>
								</div>
							</div>
							<div class="form-group row">
								<div class="col-sm-10">
									<button type="submit" class="btn btn-primary">Create Series</button>
								</div>
							</div>
						</form>
					</div>
				</div>
			</div>
		</div>
	</template>
	<template v-else-if="mode == 'annotate'">
		<component v-bind:is="annotateTool" v-bind:series="selectedSeries"></component>
	</template>
	<template v-else-if="mode == 'visualize'">
		<component v-bind:is="visualizeTool" v-bind:series="selectedSeries"></component>
	</template>
</div>
	`,
});
