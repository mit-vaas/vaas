Vue.component('annotate-default-int', {
	data: function() {
		return {
			response: null,
			imMeta: null,

			// from AnnotateMetadata
			settings: null,

			inputVal: '',
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
		$.get(this.getLabelsURL(-1), this.updateImage, 'json');
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
			$.get(this.response.URLs[0]+'&type=meta', (meta) => {
				this.imMeta = meta;
			});
		},
		prev: function() {
			if(this.response.Index < 0) {
				$.get(this.getLabelsURL(0), this.updateImage, 'json');
			} else {
				var i = this.response.Index - 1;
				$.get(this.getLabelsURL(i), this.updateImage, 'json');
			}
		},
		next: function() {
			if(this.response.Index < 0) {
				$.get(this.getLabelsURL(-1), this.updateImage, 'json');
			} else {
				var i = this.response.Index+1;
				$.get(this.getLabelsURL(i), this.updateImage, 'json');
			}
		},
		label: function(val) {
			var params = {
				id: this.series.ID,
				index: this.response.Index,
				slice: this.response.Slice,
				labels: [val],
			};
			$.ajax({
				type: "POST",
				url: '/series/int-label',
				data: JSON.stringify(params),
				processData: false,
				success: function() {
					if(this.response.Index < 0) {
						$.get(this.getLabelsURL(-1), this.updateImage, 'json');
					} else {
						var i = this.response.Index+1;
						$.get(this.getLabelsURL(i), this.updateImage, 'json');
					}
				}.bind(this),
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
			$.post('/series/update', params);
		},
	},
	template: `
<div>
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
				<template v-if="settings.NumFrames == 1">
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
		<template v-if="settings.Range > 0">
			<div v-for="i in settings.Range">
				<button v-on:click="label(i)" type="button" class="btn btn-primary">{{ i }}</button>
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
