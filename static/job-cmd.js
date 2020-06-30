Vue.component('job-cmd', {
	data: function() {
		return {
			lines: '',
		};
	},
	props: ['job'],
	created: function() {
		this.update(true);
		setInterval(this.update, 1000);
	},
	destroyed: function() {
		clearInterval(this.interval);
	},
	methods: {
		update: function(first) {
			$.get('/jobs/detail', {job_id: this.job.ID}, (lines) => {
				this.lines = lines;
				if(first || window.innerHeight + window.scrollY >= document.body.scrollHeight) {
					Vue.nextTick(() => {
						window.scrollTo(0, document.body.scrollHeight);
					});
				}
			}, 'json');
		},
	},
	template: `
<div>
	<div class="plaintext-div">
		<template v-for="line in lines">
			{{ line }}<br />
		</template>
	</div>
</div>
	`,
});
