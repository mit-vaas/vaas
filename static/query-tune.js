Vue.component('query-tune', {
	data: function() {
		return {
		tunableNodes: [],
			labelSeries: [],
			dataSeries: [],
			nodes: [],

			tuneNode: '',
			inputSeries: '',
			metricSeries: '',
			metricNode: '',

			tuneResults: null,
		};
	},
	created: function() {
		this.socket = io('/tune');
		this.socket.on('tune-result', (resp) => {
			if(this.tuneResults == null || resp.Idx >= this.tuneResults.length) {
				return;
			}
			resp.done = true;
			resp.Stats.timeMS = parseInt(resp.Stats.Time/1000000)
			Vue.set(this.tuneResults, resp.Idx, resp);
		});
	},
	destroyed: function() {
		this.socket.disconnect();
	},
	props: ['query', 'qtab'],
	methods: {
		fetch: function() {
			$.get('/tune/tunable-nodes?query_id='+this.query.ID, (nodes) => {
				this.tunableNodes = nodes;
			});
			$.get('/datasets', function(data) {
				this.dataSeries = data;
			}.bind(this));
			$.get('/labelseries', function(data) {
				this.labelSeries = data;
			}.bind(this));
			$.get('/queries/query?query_id='+this.query.ID, (query) => {
				this.nodes = [];
				for(var nodeID in query.Nodes) {
					this.nodes.push(query.Nodes[nodeID]);
				}
			});
		},
		tune: function() {
			var request = {
				NodeIDs: [parseInt(this.tuneNode)],
				Vector: this.inputSeries+'',
				MetricSeries: parseInt(this.metricSeries),
				MetricNode: parseInt(this.metricNode),
			};
			this.socket.emit('tune', request, (results) => {
				this.tuneResults = results;
			});
		},
	},
	watch: {
		qtab: function() {
			if(this.qtab != '#q-tune-panel') {
				return;
			}
			this.fetch();
		},
	},
	template: `
<div class="small-container">
	<form v-on:submit.prevent="tune">
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Nodes to Tune</label>
			<div class="col-sm-7">
				<select v-model="tuneNode" class="form-control">
					<option v-for="node in tunableNodes" :value="node.ID">{{ node.Name }}</option>
				</select>
			</div>
		</div>
			<div class="form-group row">
				<label class="col-sm-5 col-form-label">Inputs</label>
				<div class="col-sm-7">
					<select v-model="inputSeries" class="form-control">
						<option v-for="ds in dataSeries" :value="ds.ID">{{ ds.Name }}</option>
					</select>
				</div>
			</div>
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Ground Truth</label>
			<div class="col-sm-7">
				<select v-model="metricSeries" class="form-control">
					<option v-for="ds in labelSeries" :value="ds.ID">{{ ds.Name }}</option>
				</select>
			</div>
		</div>
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Output Node</label>
			<div class="col-sm-7">
				<select v-model="metricNode" class="form-control">
					<option v-for="node in nodes" :value="node.ID">{{ node.Name }}</option>
				</select>
			</div>
		</div>
		<div class="form-group row">
			<button type="submit" class="btn btn-primary">Tune</button>
		</div>
	</form>
	<table v-if="tuneResults != null" class="table">
		<thead>
			<tr>
				<th>Config</th>
				<th>Score</th>
				<th>Time</th>
			</tr>
		</thead>
		<tbody>
			<tr v-for="r in tuneResults">
				<td>{{ r.Description[0] }}</td>
				<template v-if="r.done">
					<td>{{ r.Score }}</td>
					<td>{{ r.Stats.timeMS }}ms per frame</td>
				</template>
				<template v-else>
					<td>Loading</td>
					<td>Loading</td>
				</template>
			</tr>
		</tbody>
	</table>
</div>
	`,
});