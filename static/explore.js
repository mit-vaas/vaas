Vue.component('explore-tab', {
	data: function() {
		return {
			query: '',
			selectedVideoID: '',
			queries: [],
			videos: [],

			resultRows: [],
		};
	},
	props: ['tab'],
	created: function() {
		this.fetch();
	},
	methods: {
		fetch: function() {
			$.get('/videos', function(data) {
				this.videos = data;
				if(!this.selectedVideoID) {
					this.selectedVideoID = this.videos[0].ID;
				}
			}.bind(this));
			$.get('/queries', function(data) {
				this.queries = data;
				if(!this.query) {
					this.query = this.queries[0].ID;
				}
			}.bind(this));
		},
		addMore: function() {
			var params = {
				video_id: this.selectedVideoID,
				query_id: this.query,
			};
			var i = this.resultRows.length;
			this.resultRows.push([]);

			var addOne = function(i, j) {
				$.post('/exec/test', params, function(data) {
					data['ready'] = true;
					data['clicked'] = false;
					Vue.set(this.resultRows[i], j, data);
				}.bind(this));
			}.bind(this);

			for(var j = 0; j < 4; j++) {
				this.resultRows[i].push({
					'ready': false,
				});
				addOne(i, j);
			}
		},
		test: function() {
			this.resultRows = [];
			this.addMore();
		},
		onClick: function(i, j) {
			this.resultRows[i][j].clicked = true;
		},
	},
	watch: {
		tab: function() {
			if(this.tab != '#explore-panel') {
				return;
			}
			this.fetch();
		},
	},
	template: `
<div id="explore-div">
	<div id="explore-exec-div" class="row m-1">
		<div class="col-4">
			<select v-model="query" class="form-control">
				<option v-for="query in queries" :value="query.ID">{{ query.Name }}</option>
			</select>
		</div>
		<div class="col-4">
			<select v-model="selectedVideoID" class="form-control">
				<option v-for="video in videos" :value="video.ID">{{ video.Name }}</option>
			</select>
		</div>
		<div class="col-2">
			<button v-on:click="test" type="button" class="btn btn-primary" id="explore-test-btn">Test</button>
		</div>
		<div class="col-2">
			<button type="button" class="btn btn-primary" id="explore-run-btn">Run</button>
		</div>
	</div>
	<div id="explore-results-div">
		<div v-for="(row, i) in resultRows" class="explore-results-row">
			<div v-for="(result, j) in row" class="explore-results-col">
				<template v-if="result.ready">
					<div>
						<span>{{ result.Slice.Clip.ID }}[{{ result.Slice.Start }}:{{ result.Slice.End }}]</span>
					</div>
					<img v-if="!result.clicked" v-on:click="onClick(i, j)" :src="result.PreviewURL" class="explore-result-img" />
					<video v-if="result.clicked" :width="result.Width" class="explore-result-img" controls autoplay>
						<source :src="result.URL" type="video/mp4"></source>
					</video>
				</template>
			</div>
		</div>
		<button v-if="resultRows.length > 0" v-on:click="addMore" class="btn btn-primary">More</button>
	</div>
</div>
	`,
});
