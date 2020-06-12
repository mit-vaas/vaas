Vue.component('video-import-youtube', {
	data: function() {
		return {
			name: '',
			url: '',
		};
	},
	methods: {
		click: function() {
			this.name = '';
			this.url = '';
			$('#v-youtube-modal').modal('show');
		},
		submit: function() {
			var params = {
				name: this.name,
				url: this.url,
			};
			$.post('/import/youtube', params, function() {
				$('#v-youtube-modal').modal('hide');
				this.$emit('imported');
			}.bind(this));
		},
	},
	template: `
<span>
	<button type="button" class="btn btn-primary" v-on:click=click>Import from YouTube</button>
	<div class="modal" tabindex="-1" role="dialog" id="v-youtube-modal">
		<div class="modal-dialog" role="document">
			<div class="modal-content">
				<div class="modal-body">
					<form v-on:submit.prevent="submit">
						<div class="form-group row">
							<label class="col-sm-2 col-form-label">Name</label>
							<div class="col-sm-10">
								<input class="form-control" type="text" v-model="name" />
							</div>
						</div>
						<div class="form-group row">
							<label class="col-sm-2 col-form-label">URL</label>
							<div class="col-sm-10">
								<input class="form-control" type="text" v-model="url" />
							</div>
						</div>
						<div class="form-group row">
							<div class="col-sm-10">
								<button type="submit" class="btn btn-primary">Import</button>
							</div>
						</div>
					</form>
				</div>
			</div>
		</div>
	</div>
</span>
	`,
});
