Vue.component('node-edit-bool-expr', {
	data: function() {
		return {
		};
	},
	props: ['initNode'],
	created: function() {
		try {
			var s = JSON.parse(this.initNode.Code);
		} catch(e) {}
	},
	methods: {
		save: function() {
			var code = JSON.stringify({
			});
			$.post('/queries/node?id='+this.initNode.ID, {
				code: code,
			});
		},
	},
	template: `
<div class="small-container m-2">
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
</div>
	`,
});
