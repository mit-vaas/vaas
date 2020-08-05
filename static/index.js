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

function myCall(method, endpoint, params, successFunc, completeFunc) {
	var args = {
		type: method,
		url: endpoint,
		error: function(req, status, errorMsg) {
			app.setError(errorMsg);
		},
	};
	if(params) {
		args.data = params;
		if(typeof(args.data) === 'string') {
			args.processData = false;
		}
	}
	if(successFunc) {
		args.success = successFunc;
	}
	if(completeFunc) {
		args.complete = completeFunc;
	}
	return $.ajax(args);
}

var app = new Vue({
	el: '#app',
	data: {
		tab: $('#myTab a[data-toggle="tab"].active').attr('href'),
		error: '',
	},
	methods: {
		changeTab: function(tab) {
			$('#myTab a[href="'+tab+'"]').tab('show');
			this.tab = tab;
		},
		setError: function(error) {
			this.error = error;
		},
	},
});
