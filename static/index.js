var keyHandler = null;

$(document).ready(function() {
	$('a[data-toggle="tab"]').on('shown.bs.tab', function(e) {
		var target = $(e.target).attr('href');
		if(target == '#annotate-panel') {
			myLoad('#annotate-panel', 'annotate-index.html', loadAnnotate);
		}
		app.tab = target;
	});

	$('body').keypress(function(e) {
		if(!keyHandler) {
			return;
		}
		keyHandler(e);
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
		tab: $('a[data-toggle="tab"].active').attr('href'),
	},
});
