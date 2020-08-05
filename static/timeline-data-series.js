Vue.component('timeline-data-series', {
	data: function() {
		return {
			items: [],
		};
	},
	props: ['timeline', 'series'],
	created: function() {
		this.fetchItems();
	},
	methods: {
		fetchItems: function() {
			myCall('GET', '/series/items?series_id='+this.series.ID, null, (data) => {
				this.items = data;
			});
		},
		deleteItem: function(item_id) {
			var params = {
				item_id: item_id,
			};
			myCall('POST', '/series/delete-item', params, () => {
				this.fetchItems();
			});
		},
	},
	template: `
<div>
	<h2>
		Timelines
		/
		<a href="#" v-on:click.prevent="$emit('back')">{{ timeline.Name }}</a>
		/
		{{ series.Name }}
	</h2>
	<div class="my-1">
		<video-import-local v-bind:series="series" v-on:imported="fetchItems"></video-import-local>
		<video-import-youtube v-bind:series="series" v-on:imported="fetchItems"></video-import-youtube>
	</div>
	<table class="table">
		<thead>
			<tr>
				<th>Slice</th>
				<th>Progress</th>
				<th></th>
			</tr>
		</thead>
		<tbody>
			<tr v-for="item in items">
				<td>{{ item.Slice.Segment.Name }}[{{ item.Slice.Start }}:{{ item.Slice.End }}]</td>
				<template v-if="item.Percent == 100">
					<td>Ready</td>
				</template>
				<template v-else>
					<td>{{ item.Percent }}%</td>
				</template>
				<td>
					<button v-on:click="deleteItem(item.ID)" class="btn btn-danger">Delete</button>
				</td>
			</tr>
		</tbody>
	</table>
</div>
	`,
});
