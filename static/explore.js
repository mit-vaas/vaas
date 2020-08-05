Vue.component('explore-tab', {
	data: function() {
		return {
			query: '',
			selectedVector: '',
			queries: [],
			vectors: [],

			mode: 'random',
			sequentialSegment: '',

			resultTotal: 0,
			resultRows: [],
			resultUUIDMap: {},
			detailResult: null,
			detailTool: '',
		};
	},
	props: ['tab'],
	created: function() {
		this.fetch();
		this.socket = io('/exec');
		this.socket.on('exec-result', (resp) => {
			resp.ready = true;
			resp.clicked = false;
			resp.selected = false;
			resp.progress = 0;
			if(this.resultRows.length == 0 || this.resultRows[this.resultRows.length-1].length >= 4) {
				this.resultRows.push([]);
			}
			var i = this.resultRows.length-1;
			var j = this.resultRows[i].length;
			this.resultRows[i].push(resp);
			this.resultUUIDMap[resp.UUID] = [i, j];
			this.resultTotal++;
		});
		this.socket.on('exec-progress', (resp) => {
			var i = this.resultUUIDMap[resp.UUID][0];
			var j = this.resultUUIDMap[resp.UUID][1];
			this.resultRows[i][j].progress = resp.Percent;
		});
		this.socket.on('exec-reject', () => {
			this.resultTotal++;
		});
		this.socket.on('exec-error', (error) => {
			app.setError(error);
		});
	},
	methods: {
		fetch: function() {
			myCall('GET', '/queries', null, (data) => {
				this.queries = data;
				if(!this.query && this.queries.length > 0) {
					this.query = this.queries[0].ID;
				}
			});
			myCall('GET', '/vectors', null, (data) => {
				this.vectors = data;
			});
		},
		addMore: function() {
			var params = {
				Vector: this.selectedVector+'',
				QueryID: this.query,
				Mode: this.mode,
				Count: 4,
			};
			if(this.mode == 'sequential') {
				var parts = this.sequentialSegment.split(']')[0].split('[');
				params.StartSlice = {
					Segment: {ID: parseInt(parts[0])},
				}
				if(parts.length >= 2) {
					var idx = parts[1].split(':')[0];
					params.StartSlice.Start = parseInt(idx);
				}
			}
			this.socket.emit('exec', params);
		},
		test: function() {
			this.resultRows = [];
			this.resultUUIDMap = {};
			this.resultTotal = 0;
			this.addMore();
		},
		execJob: function() {
			var params = {
				query_id: this.query,
				vector: this.selectedVector+'',
			};
			myCall('POST', '/exec/job', params);
		},
		onClick: function(i, j) {
			this.resultRows[i][j].clicked = true;
		},
		toggleResult: function(i, j) {
			var r = this.resultRows[i][j];
			r.selected = !r.selected;
			if(r.selected) {
				this.sequentialSegment = r.Slice.Segment.ID + '[' + r.Slice.Start + ']';
			}
		},
		viewDetails: function(i, j) {
			this.detailResult = this.resultRows[i][j];
			if(this.detailResult.Type == 'detection' || this.detailResult.Type == 'track') {
				this.detailTool = 'explore-detail-detection';
			}
		},
		detailBack: function() {
			this.detailResult = null;
			this.detailTool = '';
		},
	},
	computed: {
		resultPending: function() {
			var count = 0;
			this.resultRows.forEach(function(row) {
				row.forEach(function(result) {
					if(result.progress < 100) {
						count++;
					}
				});
			});
			return count;
		},
		resultCompleted: function() {
			return this.resultTotal - this.resultPending;
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
							<option v-for="query in queries" :key="query.ID" :value="query.ID">{{ query.Name }}</option>
						</select>
					</div>
				</div>
				<div class="form-group row">
					<label class="col-sm-4">Vector</label>
					<div class="col-sm-8">
						<select v-model="selectedVector" class="form-control">
							<option v-for="vector in vectors" :key="vector.ID" :value="vector.VectorStr">{{ vector.Vector | prettyVector }}</option>
						</select>
					</div>
				</div>
				<div>
					<button v-on:click="test" type="button" class="btn btn-primary">Run</button>
					<button v-on:click="execJob" type="button" class="btn btn-primary">Make Job</button>
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
					<label class="col-sm-4">From Segment</label>
					<div class="col-sm-8">
						<input type="text" class="form-control" v-model="sequentialSegment" />
					</div>
				</div>
			</div>
		</div>
		<div id="explore-results-div">
			<p v-if="resultTotal > 0">Completed {{ resultCompleted }}/{{ resultTotal }}</p>
			<div v-for="(row, i) in resultRows" class="explore-results-row">
				<div v-for="(result, j) in row" v-on:click.stop="toggleResult(i, j)" class="explore-results-col" :class="{selected: result.selected}">
					<template v-if="result.ready">
						<div>
							<span>{{ result.Slice.Segment.ID }}[{{ result.Slice.Start }}:{{ result.Slice.End }}]</span>
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
						<video v-if="result.clicked" class="explore-result-img" controls autoplay>
							<source :src="result.URL + '&type=mp4'" type="video/mp4"></source>
						</video>
						<div v-if="result.progress < 100">
							<div class="progress">
								<div
									class="progress-bar" role="progressbar" aria-valuemin="0" aria-valuemax="100"
									:aria-valuenow="result.progress"
									:style="{width: result.progress + '%'}"
									>
								</div>
							</div>
						</div>
					</template>
				</div>
			</div>
			<button v-if="resultRows.length > 0" v-on:click="addMore" class="btn btn-primary">More</button>
			<query-suggestions v-if="query != ''" v-bind:query_id="query"></query-suggestions>
		</div>
	</template>
	<template v-else>
		<div>
			<button type="button" v-on:click="detailBack" class="btn btn-primary">Back</button>
		</div>
		<component v-if="detailResult != null" v-bind:is="detailTool" v-bind:result="detailResult"></component>
	</template>
</div>
	`,
});
