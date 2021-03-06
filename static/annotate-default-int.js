Vue.component('annotate-default-int', {
	data: function() {
		return {
			response: null,
			imMeta: null,

			// from AnnotateMetadata
			settings: null,

			inputVal: '',

			// cache of unlabeled responses to use as examples
			nextCache: [],
		};
	},
	props: ['series'],
	created: function() {
		var settings = JSON.parse(this.series.AnnotateMetadata);
		if(!settings.NumFrames) {
			settings.NumFrames = 1;
		}
		if(!settings.Range) {
			settings.Range = 2;
		}
		this.settings = settings;
		myCall('GET', this.getLabelsURL(-1), null, this.updateImage);
	},
	mounted: function() {
		this.keypressHandler = (e) => {
			if(document.activeElement.tagName == 'INPUT') {
				return;
			}

			// keycode 48 through 57 are 0 through 9
			if(e.keyCode < 48 || e.keyCode > 57) {
				return;
			}
			var label = parseInt(e.keyCode) - 48;
			this.label(label);
		};
		app.$on('keypress', this.keypressHandler);
	},
	unmounted: function() {
		app.$off('keypress', this.keypressHandler);
		this.keypressHandler = null;
	},
	methods: {
		getLabelsURL: function(index) {
			return '/series/labels?id='+this.series.ID+'&nframes='+this.settings.NumFrames+'&index='+index;
		},
		updateImage: function(response) {
			this.response = response;
			this.imMeta = null;
			this.inputVal = '';
			if(this.response.Labels) {
				this.inputVal = this.response.Labels[0].toString();
			}
			myCall('GET', this.response.URLs[0]+'&type=meta', null, (meta) => {
				this.imMeta = meta;
			});
		},
		get: function(i) {
			if(i >= 0) {
				myCall('GET', this.getLabelsURL(i), null, this.updateImage);
				return;
			}
			var cacheResponse = () => {
				myCall('GET', this.getLabelsURL(-1), null, (response) => {
					this.nextCache.push(response);
				});
			};
			if(this.nextCache.length > 0) {
				cacheResponse();
				var response = this.nextCache.splice(0, 1)[0];
				this.updateImage(response);
				return
			}
			myCall('GET', this.getLabelsURL(-1), null, (response) => {
				this.updateImage(response);
				for(var j = 0; j < 8; j++) {
					cacheResponse();
				}
			});
		},
		prev: function() {
			if(this.response.Index < 0) {
				this.get(0);
			} else {
				var i = this.response.Index - 1;
				this.get(i);
			}
		},
		next: function() {
			if(this.response.Index < 0) {
				this.get(-1);
			} else {
				var i = this.response.Index+1;
				this.get(i);
			}
		},
		label: function(val) {
			var params = {
				id: this.series.ID,
				index: this.response.Index,
				slice: this.response.Slice,
				labels: [],
			};
			for(var i = params.slice.Start; i < params.slice.End; i++) {
				params.labels.push(val);
			}
			myCall('POST', '/series/int-label', JSON.stringify(params), () => {
				if(this.response.Index < 0) {
					this.get(-1);
				} else {
					var i = this.response.Index+1;
					this.get(i);
				}
			});
		},
		labelInput: function() {
			this.label(parseInt(this.inputVal));
		},
		saveSettings: function() {
			var params = {
				series_id: this.series.ID,
				annotate_metadata: JSON.stringify(this.settings),
			};
			myCall('POST', '/series/update', params);
		},
	},
	template: `
<div>
	<div>
		<p>The tool will display video samples from the configured source vector.</p>
		<p>Adjust the number of frames to sample video segments instead of individual frames.</p>
		<p>
			Enter the appropriate integer label for a sample by clicking the button or pressing 0-9 on keyboard.
			Set range to control the number of buttons; if range is 0, a text box will be provided for entering any integer.
		</p>
	</div>
	<div>
		<form class="form-inline" v-on:submit.prevent="saveSettings">
			<label class="my-1 mx-1"># Frames</label>
			<input type="text" class="form-control my-1 mx-1" v-model="settings.NumFrames" />

			<label class="my-1 mx-1">Range</label>
			<input type="text" class="form-control my-1 mx-1" v-model="settings.Range" />

			<button type="submit" class="btn btn-primary my-1 mx-1">Save Settings</button>
		</form>
	</div>
	<div>
		<template v-if="imMeta != null">
			<div :style="{
					width: imMeta.Width + 'px',
					height: imMeta.Height + 'px',
				}"
				>
				<template v-if="parseInt(settings.NumFrames) == 1">
					<img :src="response.URLs[0] + '&type=jpeg'" />
				</template>
				<template v-else>
					<video controls>
						<source :src="response.URLs[0] + '&type=mp4'" type="video/mp4"></source>
					</video>
				</template>
			</div>
		</template>
	</div>
	<div class="form-row align-items-center">
		<div class="col-auto">
			<button v-on:click="prev" type="button" class="btn btn-primary">Prev</button>
		</div>
		<div class="col-auto">
			<template v-if="response != null">
				<span v-if="response.Index < 0">[New]</span>
				<span v-else>{{ response.Index }}</span>
				<template v-if="response.Labels">
					<span>(Value: {{ response.Labels[0] }})</span>
				</template>
			</template>
		</div>
		<div class="col-auto">
			<button v-on:click="next" type="button" class="btn btn-primary">Next</button>
		</div>
		<template v-if="parseInt(settings.Range) > 0">
			<div v-for="i in parseInt(settings.Range)">
				<button v-on:click="label(i-1)" type="button" class="btn btn-primary">{{ i-1 }}</button>
			</div>
		</template>
		<template v-else>
			<div class="col-auto">
				<form class="form-inline" v-on:submit.prevent="labelInput">
					<input type="text" class="form-control" v-model="inputVal" />
					<button type="submit" class="btn btn-primary">Label</button>
				</form>
			</div>
		</template>
	</div>
</div>
	`,
});
