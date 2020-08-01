Vue.component('video-import-youtube', {
	data: function() {
		return {
			url: '',
		};
	},
	props: ['series'],
	mounted: function() {
		$('#v-youtube-modal').on('shown.bs.modal', function() {
			$('#v-youtube-name').focus();
		});
	},
	methods: {
		click: function() {
			this.url = '';
			$('#v-youtube-modal').modal('show');
		},
		submit: function() {
			var params = {
				series_id: this.series.ID,
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
