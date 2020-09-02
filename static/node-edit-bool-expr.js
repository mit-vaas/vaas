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
			myCall('POST', '/queries/node?id='+this.initNode.ID, {
				code: code,
			});
		},
	},
	template: `
<div class="small-container m-2">
	<p>
		You ought to be able to at least select between AND and OR here.
		Currently, though, this node always spits out the logical AND of all of its parents.
		Parents can be any data type, and each data type has its own semantics on truthiness (e.g. at least one detection, non-zero integer, etc.).
	</p>
	<p>Parents are evaluated from left to right. So add the fast, selective nodes as parents first.</p>
	<button v-on:click="save" type="button" class="btn btn-primary">Save</button>
</div>
	`,
});
