var keyHandler = null;

$(document).ready(function() {
	$('#myTab a[data-toggle="tab"]').on('shown.bs.tab', function(e) {
		var target = $(e.target).attr('href');
		app.tab = target;
	});

	$('body').keypress(function(e) {
		app.$emit('keypress', e);
	});
});

function myLoad(target, href, f) {
	keyHandler = null;
	$(target).html('');
	$(target).load(href + '?x=' + Math.floor(Date.now() / 1000), f);
}

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
