Vue.component('annotate-default-class', {
	data: function() {
		return {
			image: null,
		};
	},
	props: ['ls'],
	created: function() {
		$.get('/labelsets/labels?id='+this.ls.ID+'&index=-1', this.updateImage, 'json');
	},
	methods: {
		updateImage: function(image) {
			this.image = image;
		},
		prev: function() {
			if(this.image.Index < 0) {
				$.get('/labelsets/labels?id='+this.ls.ID+'&index=0', this.updateImage, 'json');
			} else {
				var i = this.image.Index - 1;
				$.get('/labelsets/labels?id='+this.ls.ID+'&index='+i, this.updateImage, 'json');
			}
		},
		next: function() {
			if(this.image.Index < 0) {
				$.get('/labelsets/labels?id='+this.ls.ID+'&index=-1', this.updateImage, 'json');
			} else {
				var i = this.image.Index+1;
				$.get('/labelsets/labels?id='+this.ls.ID+'&index='+i, this.updateImage, 'json');
			}
		},
		label: function(cls) {
			var params = {
				id: this.ls.ID,
				index: this.image.Index,
				uuid: this.image.UUID,
				labels: [cls],
			};
			$.ajax({
				type: "POST",
				url: '/labelsets/class-label',
				data: JSON.stringify(params),
				processData: false,
				success: function() {
					if(this.image.Index < 0) {
						$.get('/labelsets/labels?id='+this.ls.ID+'&index=-1', this.updateImage, 'json');
					} else {
						var i = this.image.Index+1;
						$.get('/labelsets/labels?id='+this.ls.ID+'&index='+i, this.updateImage, 'json');
					}
				}.bind(this),
			});
		},
	},
	template: `
<div>
	<div>
		<template v-if="image != null">
			<div :style="{
					width: image.Width + 'px',
					height: image.Height + 'px',
				}"
				>
				<img :src="image.URL" />
			</div>
		</template>
	</div>
	<div class="form-row align-items-center">
		<div class="col-auto">
			<button v-on:click="prev" type="button" class="btn btn-primary">Prev</button>
		</div>
		<div class="col-auto">
			<template v-if="image != null">
				<span v-if="image.Index < 0">[New]</span>
				<span v-else>{{ image.Index }}</span>
				<template v-if="image.Labels">
					<span v-if="image.Labels[0] == 1">(Positive)</span>
					<span v-else-if="image.Labels[0] == 0">(Negative)</span>
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
