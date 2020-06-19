Vue.component('node-edit-filter-detection', {
	data: function() {
		return {
			score: 0,
			classes: [],
			newClass: '',
		};
	},
	props: ['initNode'],
	created: function() {
		try {
			var s = JSON.parse(this.initNode.Code);
			this.score = s.Score;
			this.classes = s.Classes;
		} catch(e) {}
	},
	methods: {
		removeClass: function(i) {
			this.classes.splice(i, 1);
		},
		addClass: function() {
			this.classes.push(this.newClass);
			this.newClass = '';
		},
		save: function() {
			var code = JSON.stringify({
				Score: parseFloat(this.score),
				Classes: this.classes,
			});
			$.post('/node?id='+this.initNode.ID, {
				code: code,
			});
		},
	},
	template: `
<div class="small-container m-2">
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Score Threshold</label>
		<div class="col-sm-7">
			<input v-model="score" type="text" class="form-control">
			<small id="emailHelp" class="form-text text-muted">Remove detections with score lower than this threshold.</small>
		</div>
	</div>
	<div class="form-group row">
		<label class="col-sm-5 col-form-label">Classes</label>
		<div class="col-sm-7">
			<table class="table table-sm table-borderless">
				<tbody>
					<tr v-for="(cls, i) in classes">
						<td>{{ cls }}</td>
						<td><button type="button" class="btn btn-danger btn-sm" v-on:click="removeClass(i)">Remove</button></td>
					</tr>
					<tr>
						<td colspan="2">
							<form v-on:submit.prevent="addClass" class="form-inline">
								<input type="text" class="form-control" v-model="newClass" />
								<button type="submit" class="btn btn-primary btn-sm mx-2">Add</button>
							</form>
						</td>
					</tr>
				</tbody>
			</table>
		</div>
	</div>
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
</div>
	`,
});
