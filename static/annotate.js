Vue.component('annotate-tab', {
	data: function() {
		return {
			mode: 'list',
			labelSeries: [],

			allSeries: [],
			newSetFields: {},

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
			$.get('/labelseries', function(data) {
				this.labelSeries = data;
			}.bind(this));
		},
		showNewLabelSeriesModal: function() {
			$.get('/series', function(data) {
				this.allSeries = data;
				this.newSetFields = {
					name: '',
					type: 'detection',
					series: '',
				};
				$('#a-new-series-modal').modal('show');
			}.bind(this));
		},
		createSeries: function() {
			var params = {
				name: this.newSetFields.name,
				type: this.newSetFields.type,
				src: this.newSetFields.series,
			};
			$.post('/labelseries', params, function(series) {
				$('#a-new-series-modal').modal('hide');
				this.selectedSeries = series;
				this.annotateTool = 'annotate-default-' + series.DataType;
				this.mode = 'annotate';
			}.bind(this));
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
		prettyVector: function(vector) {
			var parts = [];
			vector.forEach(function(series) {
				parts.push(series.Name);
			});
			return '[' + parts.join(', ') + ']';
		},
		deleteSeries: function(series_id) {
			$.post('/series/delete', {'series_id': series_id}, function() {
				this.fetchLabelSeries(true);
			}.bind(this));
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
					<td>{{ prettyVector(series.SrcVector) }}</td>
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
										<option value="class">Class</option>
										<option value="video">Video</option>
									</select>
								</div>
							</div>
							<div class="form-group row">
								<label class="col-sm-2 col-form-label">Source</label>
								<div class="col-sm-10">
									<select v-model="newSetFields.series" class="form-control">
										<option v-for="series in allSeries" :value="series.ID">{{ series.Name }}</option>
									</select>
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
