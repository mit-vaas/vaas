Vue.component('timeline-manage', {
	data: function() {
		return {
			vectors: [],
			allSeries: [],
			dataSeries: [],
			labelSeries: [],
			outputSeries: [],
			nodes: [],
			addDataSeriesForm: {},
			addVectorForm: {},
			addOutputSeriesForm: {},

			selectedSeries: null,
		};
	},
	props: ['timeline'],
	created: function() {
		this.fetch();
		myCall('GET', '/nodes', null, (nodes) => {
			this.nodes = nodes;
		});
	},
	methods: {
		fetch: function() {
			myCall('GET', '/timeline/series?timeline_id='+this.timeline.ID, null, (data) => {
				this.dataSeries = data.DataSeries;
				this.labelSeries = data.LabelSeries;
				this.outputSeries = data.OutputSeries;
				this.allSeries = this.dataSeries.concat(this.labelSeries).concat(this.outputSeries);
			});
			myCall('GET', '/timeline/vectors?timeline_id='+this.timeline.ID, null, (data) => {
				this.vectors = data;
			});
		},
		showAddDataSeriesModal: function() {
			this.addDataSeriesForm = {
				name: '',
				dataType: '',
			};
			$(this.$refs.addDataSeriesModal).modal('show');
		},
		addDataSeries: function() {
			var params = {
				timeline_id: this.timeline.ID,
				name: this.addDataSeriesForm.name,
				data_type: this.addDataSeriesForm.dataType,
			};
			myCall('POST', '/series', params, () => {
				$(this.$refs.addDataSeriesModal).modal('hide');
				this.fetch();
			});
		},
		deleteSeries: function(series_id) {
			myCall('POST', '/series/delete', {'series_id': series_id}, () => {
				this.fetch();
			});
		},
		selectSeries: function(series) {
			this.selectedSeries = series;
		},
		deleteVector: function(vector_id) {
			myCall('POST', '/vectors/delete', {'vector_id': vector_id}, () => {
				this.fetch();
			});
		},
		exportSeries: function(series_id) {
			myCall('POST', '/series/export', {'series_id': series_id});
		},
		exportVector: function(vector_id) {
			myCall('POST', '/timelines/vectors/export', {'vector_id': vector_id});
		},

		// add vector form
		showAddVectorModal: function() {
			this.addVectorForm = {
				series: [],
				selectSeries: '',
			};
			$(this.$refs.addVectorModal).modal('show');
		},
		vectorFormAddSeries: function() {
			var series = null;
			this.allSeries.forEach((el) => {
				if(el.ID != parseInt(this.addVectorForm.selectSeries)) {
					return;
				}
				series = el;
			});
			if(!series) {
				return;
			}
			this.addVectorForm.series.push(series);
			this.addVectorForm.selectSeries = '';
		},
		addVector: function() {
			var ids = [];
			this.addVectorForm.series.forEach((el) => {
				ids.push(el.ID);
			});
			var params = {
				timeline_id: this.timeline.ID,
				series_ids: ids.join(','),
			};
			myCall('POST', '/timeline/vectors', params, () => {
				$(this.$refs.addVectorModal).modal('hide');
				this.fetch();
			});
		},
		showAddOutputSeriesModal: function() {
			this.addOutputSeriesForm = {
				node: '',
				vector: '',
			};
			$(this.$refs.addOutputSeriesModal).modal('show');
		},
		addOutputSeries: function() {
			var params = {
				node_id: this.addOutputSeriesForm.node,
				vector: this.addOutputSeriesForm.vector,
			};
			myCall('POST', '/ensure-output-series', params, () => {
				$(this.$refs.addOutputSeriesModal).modal('hide');
				this.fetch();
			});
		},
	},
	template: `
<div>
	<template v-if="selectedSeries == null">
		<h2>
			<a href="#" v-on:click.prevent="$emit('back')">Timelines</a>
			/
			{{ timeline.Name }}
		</h2>
		<p>There are three types of series:</p>
		<ul>
			<li>Data series: raw data imported into Vaas.</li>
			<li>Output series: data produced by a node in a query</li>
			<li>Label series: hand-annotated data.</li>
		</ul>
		<p>If you're just getting started, create a new Data Series below with Video type, and then Manage it to import video.</p>
		<h4>Vectors</h4>
		<p>
			<button type="button" class="btn btn-primary" v-on:click="showAddVectorModal">Add Vector</button>
			<div class="modal" tabindex="-1" role="dialog" ref="addVectorModal">
				<div class="modal-dialog" role="document">
					<div class="modal-content">
						<div class="modal-body">
							<form v-on:submit.prevent="addVector">
								<p>A vector is an ordered list of series. Add the desired series below.</p>
								<table class="table">
									<tbody>
										<tr v-for="(series, i) in addVectorForm.series">
											<td>{{ series.Name }}</td>
											<td>
												<button v-on:click="vectorFormRemoveSeries(i)" class="btn btn-danger">Remove</button>
											</td>
										</tr>
										<tr>
											<td>
												<select v-model="addVectorForm.selectSeries" class="form-control">
													<option v-for="series in allSeries" :key="series.ID" :value="series.ID">{{ series.Name }}</option>
												</select>
											</td>
											<td>
												<button type="button" class="btn btn-primary" v-on:click="vectorFormAddSeries">Add</button>
											</td>
										</tr>
									</tbody>
								</table>
								<div class="form-group row">
									<div class="col-sm-10">
										<button type="submit" class="btn btn-primary">Add Vector</button>
									</div>
								</div>
							</form>
						</div>
					</div>
				</div>
			</div>
		</p>
		<table class="table">
			<thead>
				<tr>
					<th>Name</th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				<tr v-for="vector in vectors">
					<td>{{ vector.Vector | prettyVector }}</td>
					<td>
						<button v-on:click="exportVector(vector.ID)" class="btn btn-primary">Export</button>
						<button v-on:click="deleteVector(vector.ID)" class="btn btn-danger">Delete</button>
					</td>
				</tr>
			</tbody>
		</table>
		<h4>Data Series</h4>
		<p>
			<button type="button" class="btn btn-primary" v-on:click="showAddDataSeriesModal">Add Data Series</button>
			<div class="modal" tabindex="-1" role="dialog" ref="addDataSeriesModal">
				<div class="modal-dialog" role="document">
					<div class="modal-content">
						<div class="modal-body">
							<form v-on:submit.prevent="addDataSeries">
								<div class="form-group row">
									<label class="col-sm-2 col-form-label">Name</label>
									<div class="col-sm-10">
										<input class="form-control" type="text" v-model="addDataSeriesForm.name" />
									</div>
								</div>
								<div class="form-group row">
									<label class="col-sm-2 col-form-label">Data Type</label>
									<div class="col-sm-10">
										<select v-model="addDataSeriesForm.dataType" class="form-control">
											<option value="detection">Detection</option>
											<option value="track">Track</option>
											<option value="int">Integer</option>
											<option value="video">Video</option>
											<option value="imlist">Image List</option>
											<option value="text">Text</option>
											<option value="float">Float</option>
											<option value="string">String</option>
										</select>
									</div>
								</div>
								<div class="form-group row">
									<div class="col-sm-10">
										<button type="submit" class="btn btn-primary">Add Data Series</button>
									</div>
								</div>
							</form>
						</div>
					</div>
				</div>
			</div>
		</p>
		<table class="table">
			<thead>
				<tr>
					<th>Name</th>
					<th>Type</th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				<tr v-for="series in dataSeries">
					<td>{{ series.Name }}</td>
					<td>{{ series.DataType }}</td>
					<td>
						<button v-on:click="selectSeries(series)" class="btn btn-primary">Manage</button>
						<button v-on:click="deleteSeries(series.ID)" class="btn btn-danger">Delete</button>
					</td>
				</tr>
			</tbody>
		</table>
		<h4>Label Series</h4>
		<table class="table">
			<thead>
				<tr>
					<th>Name</th>
					<th>Type</th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				<tr v-for="series in labelSeries">
					<td>{{ series.Name }}</td>
					<td>{{ series.DataType }}</td>
					<td>
						<button v-on:click="exportSeries(series.ID)" class="btn btn-primary">Export</button>
					</td>
				</tr>
			</tbody>
		</table>
		<h4>Output Series</h4>
		<p>
			<button type="button" class="btn btn-primary" v-on:click="showAddOutputSeriesModal">Add Output Series</button>
			<div class="modal" tabindex="-1" role="dialog" ref="addOutputSeriesModal">
				<div class="modal-dialog" role="document">
					<div class="modal-content">
						<div class="modal-body">
							<form v-on:submit.prevent="addOutputSeries">
								<div class="form-group row">
									<label class="col-sm-2 col-form-label">Node</label>
									<div class="col-sm-10">
										<select v-model="addOutputSeriesForm.node" class="form-control">
											<option v-for="node in nodes" :key="node.ID" :value="node.ID">{{ node.Name }}</option>
										</select>
									</div>
								</div>
								<div class="form-group row">
									<label class="col-sm-2 col-form-label">Source Vector</label>
									<div class="col-sm-10">
										<select v-model="addOutputSeriesForm.vector" class="form-control">
											<option v-for="vector in vectors" :key="vector.ID" :value="vector.Vector | strVector">{{ vector.Vector | prettyVector }}</option>
										</select>
									</div>
								</div>
								<div class="form-group row">
									<div class="col-sm-10">
										<button type="submit" class="btn btn-primary">Add Output Series</button>
									</div>
								</div>
							</form>
						</div>
					</div>
				</div>
			</div>
		</p>
		<table class="table">
			<thead>
				<tr>
					<th>Name</th>
					<th>Type</th>
				</tr>
			</thead>
			<tbody>
				<tr v-for="series in outputSeries">
					<td>{{ series.Name }}</td>
					<td>{{ series.DataType }}</td>
				</tr>
			</tbody>
		</table>
	</template>
	<template v-else>
		<timeline-data-series v-bind:timeline="timeline" v-bind:series="selectedSeries" v-on:back="selectSeries(null)"></timeline-data-series>
	</template>
</div>
	`,
});
