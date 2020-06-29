Vue.component('query-suggestions', {
	data: function() {
		return {
			suggestions: [],
			interval: null,
		};
	},
	props: ['query_id'],
	created: function() {
		this.fetchSuggestions();
		this.interval = setInterval(this.fetchSuggestions, 5000);
	},
	destroyed: function() {
		clearInterval(this.interval);
	},
	methods: {
		fetchSuggestions: function() {
			$.get('/suggestions?query_id='+this.query_id, (suggestions) => {
				this.suggestions = suggestions;
			});
		},
		applySuggestion: function(suggestion) {
			$.ajax({
				type: 'POST',
				url: '/suggestions/apply',
				data: JSON.stringify(suggestion),
				processData: false,
				success: () => {
					app.$emit('showQuery', suggestion.QueryID);
				},
			});
		},
	},
	template: `
<div>
	<table v-if="suggestions.length > 0" class="table">
		<thead>
			<tr>
				<th>Suggestion</th>
				<th>Action</th>
			</tr>
		</thead>
		<tbody>
			<tr v-for="s in suggestions">
				<td>{{ s.Text }}</td>
				<td>
					<button v-on:click="applySuggestion(s)" class="btn btn-primary btn-sm">{{ s.ActionLabel }}</button>
				</td>
			</tr>
		</tbody>
	</table>
</div>
	`,
});
