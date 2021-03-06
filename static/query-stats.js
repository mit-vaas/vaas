Vue.component('query-stats', {
	data: function() {
		return {
			stats: null,
		};
	},
	props: ['query', 'qtab'],
	methods: {
		fetchStats: function() {
			myCall('GET', '/stats?query_id='+this.query.ID, null, (stats) => {
				this.stats = stats;
			});
		},
	},
	watch: {
		qtab: function() {
			if(this.qtab != '#q-stats-panel') {
				return;
			}
			this.fetchStats();
		},
	},
	template: `
<div>
	<table v-if="stats != null" class="table">
		<thead>
			<tr>
				<th>Node</th>
				<th>Execution Time</th>
				<th>Idle Fraction</th>
				<th>Runs</th>
			</tr>
		</thead>
		<tbody>
			<tr v-for="(el, nodeID) in stats" :key="nodeID">
				<td>{{ query.Nodes[nodeID].Name }}</td>
				<td>{{ parseInt(el.Time.T/1000000) }}ms</td>
				<td>{{ el.Idle.Fraction }}</td>
				<td>{{ el.Time.Count }}</td>
			</tr>
		</tbody>
	</table>
</div>
	`,
});
