Vue.component('video-tab', {
	data: function() {
		return {
			videos: [],
		};
	},
	created: function() {
		this.fetchVideos();
		setInterval(this.fetchVideos, 1000);
	},
	methods: {
		fetchVideos: function() {
			if(getTab() != 'video-tab') {
				return;
			}
			$.get('/videos', function(data) {
				this.videos = data;
			}.bind(this));
		},
	},
	template: `
		<div>
			<div class="my-1">
				<video-import-local v-on:imported="fetchVideos"></video-import-local>
				<video-import-youtube v-on:imported="fetchVideos"></video-import-youtube>
			</div>
			<table class="table">
				<thead>
					<tr>
						<th>Name</th>
						<th>Progress</th>
					</tr>
				</thead>
				<tbody>
					<tr v-for="video in videos">
						<td>{{ video.Name }}</td>
						<template v-if="video.Percent == 100">
							<td>Ready</td>
						</template>
						<template v-else>
							<td>{{ video.Percent }}%</td>
						</template>
					</tr>
				</tbody>
			</table>
		</div>
	`,
});
