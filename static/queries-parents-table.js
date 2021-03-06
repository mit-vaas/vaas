Vue.component('queries-parents-table', {
	data: function() {
		return {
			selected: '',
			specSet: {},
		};
	},
	props: [
		'query', 'parents', 'label',

		// list of specs to hide
		'excluded',
	],
	created: function() {
		this.updateSet();
	},
	methods: {
		updateSet: function() {
			specSet = {};
			this.parents.forEach((parent) => {
				specSet[parent.Spec] = true;
			});
			if(this.excluded) {
				this.excluded.forEach((spec) => {
					specSet[spec] = true;
				});
			}
			this.specSet = specSet;
		},
		add: function() {
			this.$emit('add', this.selected);
		},
	},
	watch: {
		parents: function() {
			this.updateSet();
			this.selected = '';
		},
	},
	template: `
<table class="table table-sm">
	<thead>
		<tr><th colspan="2">{{ label }}</th></tr>
	</thead>
	<tbody>
		<tr v-for="parent in parents" :key="parent.Spec">
			<template v-if="parent.Type == 's'">
				<td>Input {{ parent.SeriesIdx }}</td>
			</template>
			<template v-else-if="parent.Type == 'n'">
				<td>{{ query.Nodes[parent.NodeID].Name }}</td>
			</template>
			<td><button type="button" class="btn btn-danger btn-sm" v-on:click="$emit('remove', parent.Spec)">Remove</button></td>
		</tr>
		<tr>
			<td>
				<select v-model="selected" class="form-control">
					<template v-for="i in query.numInputs">
						<option v-if="!specSet['s'+(i-1)]" :value="'s'+(i-1)">Input {{ i-1 }}</option>
					</template>
					<template v-for="node in query.Nodes">
						<option :key="node.ID" v-if="!specSet['n' + node.ID]" :value="'n' + node.ID">{{ node.Name }}</option>
					</template>
				</select>
			</td>
			<td><button type="button" class="btn btn-success btn-sm" v-on:click="add">Add</button></td>
		</tr>
	</tbody>
</table>
	`,
});
