Vue.component('node-edit-text', {
	data: function() {
		return {
			node: {},
		};
	},
	props: ['initNode'],
	created: function() {
		this.node = JSON.parse(JSON.stringify(this.initNode));
	},
	methods: {
		autoindent: function(e) {
			var el = e.target;
			if(e.keyCode == 9) {
				// tab
				e.preventDefault();
				var start = el.selectionStart;
				var prefix = this.node.Code.substring(0, start);
				var suffix = this.node.Code.substring(el.selectionEnd);
				this.node.Code = prefix + '\t' + suffix;
				el.selectionStart = start+1;
				el.selectionEnd = start+1;
			} else if(e.keyCode == 13) {
				// enter
				e.preventDefault();
				var start = el.selectionStart;
				var prefix = this.node.Code.substring(0, start);
				var suffix = this.node.Code.substring(el.selectionEnd);
				var prevLine = this.node.Code.lastIndexOf('\n', start);

				var spacing = '';
				if(prevLine != -1) {
					for(var i = prevLine+1; i < start; i++) {
						var char = this.node.Code[i];
						if(char != '\t' && char != ' ') {
							break;
						}
						spacing += char;
					}
				}

				this.node.Code = prefix + '\n' + spacing + suffix;
				el.selectionStart = start+1+spacing.length;
				el.selectionEnd = el.selectionStart;
			}
		},
		save: function() {
			$.post('/node?id='+this.node.ID, {
				code: this.node.Code,
			});
		},
	},
	template: `
<div id="n-edit-text-div">
	<div id="n-edit-text-code-div">
		<textarea v-model="node.Code" v-on:keydown="autoindent($event)" id="n-edit-text-code" placeholder="Your Code Here"></textarea>
	</div>
	<div class="m-1">
		<button v-on:click="save" type="button" class="btn btn-primary btn-sm" id="n-edit-text-save">Save</button>
	</div>
</div>
	`,
});
