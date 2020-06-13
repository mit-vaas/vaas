Vue.component('annotate-tab', {
	data: function() {
		return {
			mode: 'list',
			labelSets: [],

			videos: [],
			newSetFields: {},

			annotateTool: '',
			visualizeTool: '',
			selectedSet: null,
		};
	},
	props: ['tab'],
	created: function() {
		this.fetchLabelSets(true);
		setInterval(this.fetchLabelSets(), 5000);
	},
	methods: {
		fetchLabelSets: function(force) {
			if(!force && this.tab != '#annotate-panel') {
				return;
			}
			$.get('/labelsets', function(data) {
				this.labelSets = data;
			}.bind(this));
		},
		showNewLabelSetModal: function() {
			$.get('/videos', function(data) {
				this.videos = data;
				this.newSetFields = {
					name: '',
					type: 'detection',
					video: '',
				};
				$('#a-new-ls-modal').modal('show');
			}.bind(this));
		},
		createLabelSet: function() {
			var params = {
				name: this.newSetFields.name,
				type: this.newSetFields.type,
				src: this.newSetFields.video,
			};
			$.post('/labelsets', params, function(ls) {
				$('#a-new-ls-modal').modal('hide');
				this.selectedSet = ls;
				this.annotateTool = 'annotate-default-' + ls.Type;
				this.mode = 'annotate';
			}.bind(this));
		},
		annotateLabels: function(ls) {
			this.selectedSet = ls;
			this.annotateTool = 'annotate-default-' + ls.Type;
			this.mode = 'annotate';
		},
		visualizeLabels: function(ls) {
			this.selectedSet = ls;
			this.visualizeTool = 'annotate-visualize';
			this.mode = 'visualize';
		},
	},
	watch: {
		tab: function() {
			if(this.tab != '#annotate-panel') {
				return;
			}
			this.mode = 'list';
			this.fetchLabelSets(true);
		},
	},
	template: `
<div>
	<template v-if="mode == 'list'">
		<div class="my-1">
			<h3>Label Sets</h3>
		</div>
		<div class="my-1">
			<button v-on:click="showNewLabelSetModal" class="btn btn-primary">New Label Set</button>
		</div>
		<table class="table">
			<thead>
				<tr>
					<th>Name</th>
					<th>Source Video</th>
					<th>Type</th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				<tr v-for="ls in labelSets">
					<td>{{ ls.Name }}</td>
					<td>{{ ls.SrcVideo.Name }}</td>
					<td>{{ ls.Type }}</td>
					<td>
						<button v-on:click="annotateLabels(ls)" class="btn btn-primary btn-sm">Annotate</button>
						<button v-on:click="visualizeLabels(ls)" class="btn btn-primary btn-sm">Visualize</button>
					</td>
				</tr>
			</tbody>
		</table>
		<div class="modal" tabindex="-1" role="dialog" id="a-new-ls-modal">
			<div class="modal-dialog" role="document">
				<div class="modal-content">
					<div class="modal-body">
						<form v-on:submit.prevent="createLabelSet">
							<div class="form-group row">
								<label class="col-sm-2 col-form-label">Name</label>
								<div class="col-sm-10">
									<input v-model="newSetFields.name" class="form-control" type="text" />
								</div>
							</div>
							<div class="form-group row">
								<label class="col-sm-2 col-form-label">Type</label>
								<div class="col-sm-10">
									<select v-model="newSetFields.type" class="form-control">
										<option value="detection">Detection</option>
										<option value="track">Track</option>
										<option value="class">Class</option>
										<option value="video">Video</option>
									</select>
								</div>
							</div>
							<div class="form-group row">
								<label class="col-sm-2 col-form-label">Source Video</label>
								<div class="col-sm-10">
									<select v-model="newSetFields.video" class="form-control">
										<option v-for="video in videos" :value="video.ID">{{ video.Name }}</option>
									</select>
								</div>
							</div>
							<div class="form-group row">
								<div class="col-sm-10">
									<button type="submit" class="btn btn-primary">Create Label Set</button>
								</div>
							</div>
						</form>
					</div>
				</div>
			</div>
		</div>
	</template>
	<template v-else-if="mode == 'annotate'">
		<component v-bind:is="annotateTool" v-bind:ls="selectedSet"></component>
	</template>
	<template v-else-if="mode == 'visualize'">
		<component v-bind:is="visualizeTool" v-bind:ls="selectedSet"></component>
	</template>
</div>
	`,
});
