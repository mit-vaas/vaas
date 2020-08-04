$(document).ready(function() {
	$('#myTab a[data-toggle="tab"]').on('shown.bs.tab', function(e) {
		var target = $(e.target).attr('href');
		app.tab = target;
	});

	$('body').keypress(function(e) {
		app.$emit('keypress', e);
	});
});

Vue.filter('prettyVector', function (vector) {
	var parts = [];
	vector.forEach((series) => {
		parts.push(series.Name);
	});
	return '[' + parts.join(', ') + ']';
});

var app = new Vue({
	el: '#app',
	data: {
		tab: $('#myTab a[data-toggle="tab"].active').attr('href'),
	},
	methods: {
		changeTab: function(tab) {
			$('#myTab a[href="'+tab+'"]').tab('show');
			this.tab = tab;
		},
	},
});
