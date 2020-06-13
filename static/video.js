Vue.component('video-tab', {
	data: function() {
		return {
			videos: [],
		};
	},
	props: ['tab'],
	created: function() {
		this.fetchVideos(true);
		setInterval(this.fetchVideos, 1000);
	},
	methods: {
		fetchVideos: function(force) {
			if(!force && this.tab != '#video-panel') {
				return;
			}
			$.get('/videos', function(data) {
				this.videos = data;
			}.bind(this));
		},
		deleteVideo: function(video_id) {
			$.post('/videos/delete', {'video_id': video_id}, function() {
				this.fetchVideos();6
			}.bind(this));
		},
	},
	watch: {
		tab: function() {
			if(this.tab != '#video-panel') {
				return;
			}
			this.fetchVideos(true);
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
				<th></th>
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
				<td>
					<button v-on:click="deleteVideo(video.ID)" class="btn btn-danger">Delete</button>
				</td>
			</tr>
		</tbody>
	</table>
</div>
	`,
});
