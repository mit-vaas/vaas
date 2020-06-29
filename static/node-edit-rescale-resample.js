Vue.component('node-edit-rescale-resample', {
	data: function() {
		return {
			width: '0',
			height: '0',
			freq: '1',

			labelSeries: [],
			dataSeries: [],
			nodes: [],

			inputSeries: '',
			metricSeries: '',
			metricNode: '',

			tuneResults: null,
		};
	},
	props: ['initNode'],
	created: function() {
		try {
			var s = JSON.parse(this.initNode.Code);
			this.width = s.Width;
			this.height = s.Height;
			this.freq = s.Freq;
		} catch(e) {}
		$.get('/datasets', function(data) {
			this.dataSeries = data;
		}.bind(this));
		$.get('/labelseries', function(data) {
			this.labelSeries = data;
		}.bind(this));
		$.get('/queries/query?query_id='+this.initNode.QueryID, (query) => {
			this.nodes = [];
			for(var nodeID in query.Nodes) {
				this.nodes.push(query.Nodes[nodeID]);
			}
		})

		this.socket = io('/nodes/rescale-resample');
		this.socket.on('tune-result', (resp) => {
			if(this.tuneResults == null || resp.CfgIdx >= this.tuneResults.length) {
				return;
			}
			let result = this.tuneResults[resp.CfgIdx];
			Vue.set(result, 'done', true);
			Vue.set(result, 'Score', resp.Score);
			Vue.set(result, 'Stats', resp.Stats);
			Vue.set(result.Stats, 'timeMS', parseInt(result.Stats.Time/1000000));
		});
	},
	destroyed: function() {
		this.socket.disconnect();
	},
	methods: {
		save: function() {
			var code = JSON.stringify({
				Width: parseInt(this.width),
				Height: parseInt(this.height),
				Freq: parseInt(this.freq),
			});
			$.post('/queries/node?id='+this.initNode.ID, {
				code: code,
			});
		},
		optimize: function() {
			var request = {
				NodeID: parseInt(this.initNode.ID),
				Vector: this.inputSeries+'',
				MetricSeries: parseInt(this.metricSeries),
				MetricNode: parseInt(this.metricNode),
			};
			this.socket.emit('tune', request, (cfgs) => {
				this.tuneResults = cfgs;
			});
		},
	},
	template: `
<div class="small-container m-2">
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Output Width</label>
		<div class="col-sm-7">
			<input v-model="width" type="text" class="form-control">
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Output Height</label>
		<div class="col-sm-7">
			<input v-model="height" type="text" class="form-control">
		</div>
	</div>
		<div class="form-group row">
			<label class="col-sm-5 col-form-label">Re-sample Rate</label>
			<div class="col-sm-7">
				<input v-model="freq" type="text" class="form-control">
				<small id="emailHelp" class="form-text text-muted">
					This rate is measured relative to the query input rate (not the parent).
					For example, "4" would downsample 4x from the raw data capture rate.
				</small>
			</div>
		</div>
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
	<form v-on:submit.prevent="optimize" class="form-inline my-2">
		<label>Optimize</label>
		<select v-model="inputSeries" class="form-control mx-2">
			<option v-for="ds in dataSeries" :value="ds.ID">{{ ds.Name }}</option>
		</select>
		<select v-model="metricSeries" class="form-control mx-2">
			<option v-for="ds in labelSeries" :value="ds.ID">{{ ds.Name }}</option>
		</select>
		<select v-model="metricNode" class="form-control mx-2">
			<option v-for="node in nodes" :value="node.ID">{{ node.Name }}</option>
		</select>
		<button type="submit" class="btn btn-primary mx-2">Select</button>
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
			<tr v-for="cfg in tuneResults">
				<td>{{ cfg.Width }}x{{ cfg.Height }} at rate {{ cfg.Freq }}</td>
				<template v-if="cfg.done">
					<td>{{ cfg.Score }}</td>
					<td>{{ cfg.Stats.timeMS }}ms per frame</td>
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
