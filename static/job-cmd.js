Vue.component('job-cmd', {
	data: function() {
		return {
			lines: '',
		};
	},
	props: ['job'],
	created: function() {
		$.get('/jobs/detail', {job_id: this.job.ID}, (lines) => {
			this.lines = lines.join("\n");
		}, 'json');
	},
	template: `
<div>
	<pre>{{ lines }}</pre>
</div>
	`,
});
