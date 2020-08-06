Vue.component('jobs-tab', {
	data: function() {
		return {
			jobs: [],
			selectedJob: null,
		};
	},
	props: ['tab'],
	created: function() {
		this.fetchJobs(true);
		setInterval(this.fetchJobs(), 5000);
	},
	methods: {
		fetchJobs: function(force) {
			if(!force && this.tab != '#jobs-panel') {
				return;
			}
			myCall('GET', '/jobs', null, (jobs) => {
				this.jobs = jobs;
			});
		},
		selectJob: function(job) {
			this.selectedJob = job;
		},
		clearJobs: function() {
			myCall('POST', '/jobs/clear', null, () => {
				this.fetchJobs(true);
			});
		},
	},
	watch: {
		tab: function() {
			if(this.tab != '#jobs-panel') {
				return;
			}
			this.selectedJob = null;
			this.fetchJobs(true);
		},
	},
	template: `
<div>
	<template v-if="selectedJob == null">
		<div class="my-1">
			<h3>Jobs</h3>
			<button type="button" class="btn btn-danger" v-on:click="clearJobs">Clear Jobs</button>
		</div>
		<table class="table">
			<thead>
				<tr>
					<th>ID</th>
					<th>Name</th>
					<th>Status</th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				<tr v-for="job in jobs">
					<td>{{ job.ID }}</td>
					<td>{{ job.Name }}</td>
					<td>{{ job.Status }}</td>
					<td>
						<button v-on:click="selectJob(job)" class="btn btn-primary btn-sm">Details</button>
					</td>
				</tr>
			</tbody>
		</table>
	</template>
	<template v-else>
		<component v-bind:is="'job-' + selectedJob.Type" v-bind:job="selectedJob"></component>
	</template>
</div>
	`,
});
