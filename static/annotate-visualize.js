Vue.component('annotate-visualize', {
	data: function() {
		return {
			curData: null,

			// user clicked the video and we're playing it?
			playing: false,

			clipIndex: 0,
			frameIndex: 0,
		};
	},
	props: ['ls'],
	created: function() {
		$.get('/labelsets/visualize?id='+this.ls.ID, this.update, 'json');
	},
	methods: {
		update: function(data) {
			this.curData = data;
		},
		playVideo: function() {
			this.playing = true;
		},
		prevClip: function() {

		},
		nextClip: function() {

		},
		prev: function() {

		},
		next: function() {

		},
	},
	template: `
<div>
	<div>
		<template v-if="curData != null">
			<div :style="{
					width: curData.Width + 'px',
					height: curData.Height + 'px',
				}"
				>
				<template v-if="this.playing">
					<video :width="curData.Width" :height="curData.Height" controls autoplay>
						<source :src="curData.URL" type="video/mp4">
					</video>
				</template>
				<template v-else>
					<img :src="curData.PreviewURL" v-on:click="playVideo" />
				</template>
			</div>
		</template>
	</div>
	<div class="form-row align-items-center">
		<div class="col-auto">
			<button v-on:click="prevClip" type="button" class="btn btn-primary">Prev Clip</button>
		</div>
			<div class="col-auto">
				<button v-on:click="prev" type="button" class="btn btn-primary">Prev</button>
			</div>
		<div class="col-auto">
			<template v-if="curData != null">
				{{ curData.Index }}
			</template>
		</div>
		<div class="col-auto">
			<button v-on:click="next" type="button" class="btn btn-primary">Next</button>
		</div>
		<div class="col-auto">
			<button v-on:click="nextClip" type="button" class="btn btn-primary">Next Clip</button>
		</div>
	</div>
</div>
	`,
});
