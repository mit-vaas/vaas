Vue.component('timelines-tab', {
	data: function() {
		return {
			timelines: [],
			addTimelineForm: {},
			selectedTimeline: null,

			importTimelineForm: {},
		};
	},
	props: ['tab'],
	created: function() {
		this.fetchTimelines(true);
		setInterval(this.fetchTimelines, 1000);
	},
	methods: {
		fetchTimelines: function(force) {
			if(!force && this.tab != '#timelines-panel') {
				return;
			}
			myCall('GET', '/timelines', null, (data) => {
				this.timelines = data;
			});
		},
		showAddTimelineModal: function() {
			this.addTimelineForm = {
				name: '',
			};
			$(this.$refs.addTimelineModal).modal('show');
		},
		addTimeline: function() {
			var params = {
				name: this.addTimelineForm.name,
			};
			myCall('POST', '/timelines', params, () => {
				$(this.$refs.addTimelineModal).modal('hide');
				this.fetchTimelines(true);
			});
		},
		deleteTimeline: function(timelineID) {
			var params = {
				timeline_id: timelineID,
			};
			myCall('POST', '/timelines/delete', params, () => {
				this.fetchTimelines(true);
			});
		},
		selectTimeline: function(timeline) {
			this.selectedTimeline = timeline;
		},
	},
	watch: {
		tab: function() {
			if(this.tab != '#timelines-panel') {
				return;
			}
			this.fetchTimelines(true);
		},
	},
	template: `
<div>
	<template v-if="selectedTimeline == null">
		<p>
			A Timeline is a segmented "time" axis shared by several series, each of which associate some data to each timestep.
			Typically, the time axis is based on a video dataset, where the axis has one segment for each video clip, and within each segment, it has one timestep for each video frame.
			Then, the dataset itself is a video series that associates an image with each timestep, and derivative series can be produced, e.g., a series associating a set of object detections with each frame.
		</p>
		<p>To get started with Vaas, add a Timeline below, and then press Manage on it to add some video data.</p>
		<div class="my-1">
			<button type="button" class="btn btn-primary" v-on:click="showAddTimelineModal">Add Timeline</button>
			<div class="modal" tabindex="-1" role="dialog" ref="addTimelineModal">
				<div class="modal-dialog" role="document">
					<div class="modal-content">
						<div class="modal-body">
							<form v-on:submit.prevent="addTimeline">
								<div class="form-group row">
									<label class="col-sm-2 col-form-label">Name</label>
									<div class="col-sm-10">
										<input class="form-control" type="text" v-model="addTimelineForm.name" />
									</div>
								</div>
								<div class="form-group row">
									<div class="col-sm-10">
										<button type="submit" class="btn btn-primary">Add Timeline</button>
									</div>
								</div>
							</form>
						</div>
					</div>
				</div>
			</div>
			<import-from-export-modal></import-from-export-modal>
		</div>
		<table class="table">
			<thead>
				<tr>
					<th>Name</th>
					<th># Data Series</th>
					<th># Label Series</th>
					<th># Output Series</th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				<tr v-for="timeline in timelines">
					<td>{{ timeline.Name }}</td>
					<td>{{ timeline.NumDataSeries }}</td>
					<td>{{ timeline.NumLabelSeries }}</td>
					<td>{{ timeline.NumOutputSeries }}</td>
					<td>
						<button v-on:click="selectTimeline(timeline)" class="btn btn-primary">Manage</button>
						<button v-if="timeline.CanDelete" v-on:click="deleteTimeline(timeline.ID)" class="btn btn-danger">Delete</button>
					</td>
				</tr>
			</tbody>
		</table>
	</template>
	<template v-else>
		<timeline-manage v-bind:timeline="selectedTimeline" v-on:back="selectTimeline(null)"></timeline-manage>
	</template>
</div>
	`,
});
