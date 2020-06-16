Vue.component('query-tab', {
	data: function() {
		return {
			queries: [],
			nodes: [],
			newQueryFields: {},
		};
	},
	props: ['tab'],
	created: function() {
		this.fetchQueries(true);
		setInterval(this.fetchQueries, 5000);
	},
	methods: {
		fetchQueries: function(force) {
			if(!force && this.tab != '#query-panel') {
				return;
			}
			$.get('/queries', function(queries) {
				queries.forEach(function(query) {
					outputsStr = [];
					query.Outputs.forEach(function(row) {
						rowStr = [];
						row.forEach(function(output) {
							if(output.Type == 'n') {
								rowStr.push(output.Node.Name);
							} else if(output.Type) {
								rowStr.push('Input[' + output.SeriesIdx + ']');
							} else {
								rowStr.push('unknown');
							}
						});
						outputsStr.push('[' + rowStr.join(', ') + ']');
					});
					query.outputs = outputsStr.join('\n');
				});
				this.queries = queries;
			}.bind(this));
		},
		showNewQueryModal: function() {
			this.newQueryFields.name = '';
			this.newQueryFields.outputs = '';
			$.get('/nodes', function(nodes) {
				this.nodes = nodes;
				$('#q-new-query-modal').modal('show');
			}.bind(this));
		},
		createQuery: function() {
			var params = {
				name: this.newQueryFields.name,
				outputs: this.newQueryFields.outputs,
			};
			$.post('/queries', params, function() {
				$('#q-new-query-modal').modal('hide');
				this.fetchQueries();
			}.bind(this));
		},
	},
	watch: {
		tab: function() {
			if(this.tab != '#query-panel') {
				return;
			}
			this.fetchQueries(true);
		},
	},
	template: `
<div>
	<div class="my-1">
		<button type="button" class="btn btn-primary" v-on:click=showNewQueryModal>New Query</button>
	</div>
	<table class="table">
		<thead>
			<tr>
				<th>Name</th>
				<th>Outputs</th>
			</tr>
		</thead>
		<tbody>
			<tr v-for="query in queries">
				<td>{{ query.Name }}</td>
				<td>{{ query.outputs }}</td>
			</tr>
		</tbody>
	</table>
	<div class="modal" tabindex="-1" role="dialog" id="q-new-query-modal">
		<div class="modal-dialog" role="document">
			<div class="modal-content">
				<div class="modal-body">
					<form v-on:submit.prevent="createQuery">
						<div class="form-group row">
							<label class="col-sm-2 col-form-label">Name</label>
							<div class="col-sm-10">
								<input v-model="newQueryFields.name" class="form-control" type="text" />
							</div>
						</div>
						<div class="form-group row">
							<label class="col-sm-2 col-form-label">Outputs</label>
							<div class="col-sm-10">
								<input v-model="newQueryFields.outputs" class="form-control" type="text" placeholder="e.g. 's0,n1'" />
							</div>
						</div>
						<div class="form-group row">
							<div class="col-sm-10">
								<button type="submit" class="btn btn-primary">Create Query</button>
							</div>
						</div>
					</form>
				</div>
			</div>
		</div>
	</div>
</div>
	`,
});
