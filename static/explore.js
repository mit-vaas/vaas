Vue.component('explore-tab', {
	data: function() {
		return {
			query: '',
			selectedVideoID: '',
			queries: [],
			videos: [],

			mode: 'random',
			sequentialClip: '',

			resultRows: [],
			detailResult: null,
			detailTool: '',
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
				VideoID: this.selectedVideoID,
				QueryID: this.query,
				Mode: this.mode,
				Count: 4,
			};
			if(this.mode == 'sequential') {
				var parts = this.sequentialClip.split(']')[0].split('[');
				params.StartSlice = {
					Clip: {ID: parseInt(parts[0])},
				}
				if(parts.length >= 2) {
					var idx = parts[1].split(':')[0];
					params.StartSlice.Start = parseInt(idx);
				}
			}
			var i = this.resultRows.length;
			var row = [];
			for(var j = 0; j < 4; j++) {
				row.push({
					ready: false,
					selected: false,
				});
			}
			this.resultRows.push(row);
			$.ajax({
				type: "POST",
				url: '/exec/test',
				data: JSON.stringify(params),
				success: function(data) {
					data.forEach(function(el) {
						el.ready = true;
						el.clicked = false;
						el.selected = false;
					});
					Vue.set(this.resultRows, i, data)
				}.bind(this),
			});
		},
		test: function() {
			this.resultRows = [];
			this.addMore();
		},
		onClick: function(i, j) {
			this.resultRows[i][j].clicked = true;
		},
		toggleResult: function(i, j) {
			var r = this.resultRows[i][j];
			r.selected = !r.selected;
			if(r.selected) {
				this.sequentialClip = r.Slice.Clip.ID + '[' + r.Slice.Start + ']';
			}
		},
		viewDetails: function(i, j) {
			this.detailResult = this.resultRows[i][j];
			if(this.detailResult.Type == 'detection' || this.detailResult.Type == 'track') {
				this.detailTool = 'explore-detail-detection';
			}
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
	<template v-if="detailResult == null">
		<div id="explore-exec-div" class="m-1">
			<div>
				<div class="form-group row">
					<label class="col-sm-4">Query</label>
					<div class="col-sm-8">
						<select v-model="query" class="form-control">
							<option v-for="query in queries" :value="query.ID">{{ query.Name }}</option>
						</select>
					</div>
				</div>
				<div class="form-group row">
					<label class="col-sm-4">Video</label>
					<div class="col-sm-8">
						<select v-model="selectedVideoID" class="form-control">
							<option v-for="video in videos" :value="video.ID">{{ video.Name }}</option>
						</select>
					</div>
				</div>
				<div>
					<button v-on:click="test" type="button" class="btn btn-primary">Run</button>
					<button v-on:click="test" type="button" class="btn btn-primary">Make Job</button>
				</div>
			</div>
			<div>
				<h3>Mode</h3>
				<div class="form-check">
					<input class="form-check-input" type="radio" value="random" v-model="mode" />
					<label class="form-check-label">Random</label>
				</div>
				<div class="form-check">
					<input class="form-check-input" type="radio" value="sequential" v-model="mode" />
					<label class="form-check-label">Sequential</label>
				</div>
				<div class="form-group row">
					<label class="col-sm-4">From Clip</label>
					<div class="col-sm-8">
						<input type="text" class="form-control" v-model="sequentialClip" />
					</div>
				</div>
			</div>
		</div>
		<div id="explore-results-div">
			<div v-for="(row, i) in resultRows" class="explore-results-row">
				<div v-for="(result, j) in row" v-on:click.stop="toggleResult(i, j)" class="explore-results-col" :class="{selected: result.selected}">
					<template v-if="result.ready">
						<div>
							<span>{{ result.Slice.Clip.ID }}[{{ result.Slice.Start }}:{{ result.Slice.End }}]</span>
							<span v-on:click.stop="viewDetails(i, j)">
								<button type="button" class="btn btn-outline-dark">
									<svg class="bi bi-arrow-bar-right" width="1em" height="1em" viewBox="0 0 16 16" fill="currentColor" xmlns="http://www.w3.org/2000/svg">
									  <path fill-rule="evenodd" d="M10.146 4.646a.5.5 0 0 1 .708 0l3 3a.5.5 0 0 1 0 .708l-3 3a.5.5 0 0 1-.708-.708L12.793 8l-2.647-2.646a.5.5 0 0 1 0-.708z"/>
									  <path fill-rule="evenodd" d="M6 8a.5.5 0 0 1 .5-.5H13a.5.5 0 0 1 0 1H6.5A.5.5 0 0 1 6 8zm-2.5 6a.5.5 0 0 1-.5-.5v-11a.5.5 0 0 1 1 0v11a.5.5 0 0 1-.5.5z"/>
									</svg>
								</button>
							</span>
						</div>
						<img v-if="!result.clicked" v-on:click.stop="onClick(i, j)" :src="result.PreviewURL" class="explore-result-img" />
						<video v-if="result.clicked" :width="result.Width" class="explore-result-img" controls autoplay>
							<source :src="result.URL + '&type=mp4'" type="video/mp4"></source>
						</video>
					</template>
				</div>
			</div>
			<button v-if="resultRows.length > 0" v-on:click="addMore" class="btn btn-primary">More</button>
		</div>
	</template>
	<template v-else>
		<component v-if="detailResult != null" v-bind:is="detailTool" v-bind:result="detailResult"></component>
	</template>
</div>
	`,
});
