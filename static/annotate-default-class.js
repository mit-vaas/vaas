Vue.component('annotate-default-class', {
	data: function() {
		return {
			response: null,
			imMeta: null,
		};
	},
	props: ['series'],
	created: function() {
		$.get('/series/labels?id='+this.series.ID+'&index=-1', this.updateImage, 'json');
	},
	methods: {
		updateImage: function(response) {
			this.response = response;
			this.imMeta = null;
			$.get(this.response.URLs[0]+'&type=meta', (meta) => {
				this.imMeta = meta;
			});
		},
		prev: function() {
			if(this.response.Index < 0) {
				$.get('/series/labels?id='+this.series.ID+'&index=0', this.updateImage, 'json');
			} else {
				var i = this.response.Index - 1;
				$.get('/series/labels?id='+this.series.ID+'&index='+i, this.updateImage, 'json');
			}
		},
		next: function() {
			if(this.response.Index < 0) {
				$.get('/series/labels?id='+this.series.ID+'&index=-1', this.updateImage, 'json');
			} else {
				var i = this.response.Index+1;
				$.get('/series/labels?id='+this.series.ID+'&index='+i, this.updateImage, 'json');
			}
		},
		label: function(cls) {
			var params = {
				id: this.series.ID,
				index: this.response.Index,
				slice: this.response.Slice,
				labels: [cls],
			};
			$.ajax({
				type: "POST",
				url: '/series/class-label',
				data: JSON.stringify(params),
				processData: false,
				success: function() {
					if(this.response.Index < 0) {
						$.get('/series/labels?id='+this.series.ID+'&index=-1', this.updateImage, 'json');
					} else {
						var i = this.response.Index+1;
						$.get('/series/labels?id='+this.series.ID+'&index='+i, this.updateImage, 'json');
					}
				}.bind(this),
			});
		},
	},
	template: `
<div>
	<div>
		<template v-if="imMeta != null">
			<div :style="{
					width: imMeta.Width + 'px',
					height: imMeta.Height + 'px',
				}"
				>
				<img :src="response.URLs[0] + '&type=jpeg'" />
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
					<span v-if="response.Labels[0] == 1">(Positive)</span>
					<span v-else-if="response.Labels[0] == 0">(Negative)</span>
				</template>
			</template>
		</div>
		<div class="col-auto">
			<button v-on:click="next" type="button" class="btn btn-primary">Next</button>
		</div>
		<div class="col-auto">
			<button v-on:click="label(1)" type="button" class="btn btn-primary">Positive</button>
		</div>
		<div class="col-auto">
			<button v-on:click="label(0)" type="button" class="btn btn-primary">Negative</button>
		</div>
	</div>
</div>
	`,
});
