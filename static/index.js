var keyHandler = null;

$(document).ready(function() {
	$('a[data-toggle="tab"]').on('shown.bs.tab', function(e) {
		var target = $(e.target).attr('href');
		if(target == '#video-panel') {
			myLoad('#video-panel', 'video-index.html', loadVideo);
		} else if(target == '#annotate-panel') {
			myLoad('#annotate-panel', 'annotate-index.html', loadAnnotate);
		} else if(target == '#query-panel') {
			myLoad('#query-panel', 'query.html', loadQuery);
		}
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
	$(target).load(href + '?x=' + Math.floor(Date.now() / 1000), f);
}
