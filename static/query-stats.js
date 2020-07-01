Vue.component('query-stats', {
	data: function() {
		return {
			stats: null,
		};
	},
	props: ['query', 'qtab'],
	methods: {
		fetchStats: function() {
			$.get('/stats?query_id='+this.query.ID, (stats) => {
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
				<th>Runs</th>
			</tr>
		</thead>
		<tbody>
			<tr v-for="(el, nodeID) in stats">
				<td>{{ query.Nodes[nodeID].Name }}</td>
				<td>{{ parseInt(el.Time/1000000) }}ms</td>
				<td>{{ el.Count }}</td>
			</tr>
		</tbody>
	</table>
</div>
	`,
});
