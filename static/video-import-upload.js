Vue.component('video-import-upload', {
	data: function() {
		return {
			file: null,
			percent: null,
		};
	},
	props: ['series'],
	mounted: function() {
		$(this.$refs.modal).on('shown.bs.modal', () => {
			$(this.$refs.modal).focus();
		});
	},
	methods: {
		click: function() {
			this.file = null;
			this.percent = null;
			$(this.$refs.modal).modal('show');
		},
		onFileChange: function(event) {
			this.file = event.target.files[0];
		},
		submit: function() {
			var data = new FormData();
			data.append('file', this.file);
			data.append('upload_file', true);
			this.percent = null;
			$.ajax({
				type: 'POST',
				url: '/import/upload?series_id='+this.series.ID,
				error: function(req, status, errorMsg) {
					app.setError(errorMsg);
				},
				data: data,
				processData: false,
				contentType: false,
				xhr: () => {
					var xhr = new window.XMLHttpRequest();
					xhr.upload.addEventListener('progress', (e) => {
						if(!e.lengthComputable) {
							return;
						}
						this.percent = parseInt(e.loaded * 100 / e.total);
					});
					return xhr;
				},
				success: () => {
					$(this.$refs.modal).modal('hide');
					this.$emit('imported');
				},
			});
		},
	},
	template: `
<span>
	<button type="button" class="btn btn-primary" v-on:click=click>Upload Video</button>
	<div class="modal" tabindex="-1" role="dialog" ref="modal">
		<div class="modal-dialog" role="document">
			<div class="modal-content">
				<div class="modal-body">
					<form v-on:submit.prevent="submit">
						<div class="form-group row">
							<label class="col-sm-2 col-form-label">File</label>
							<div class="col-sm-10">
								<input class="form-control" type="file" @change="onFileChange" />
							</div>
						</div>
						<div class="form-group row">
							<div class="col-sm-10">
								<button type="submit" class="btn btn-primary">Upload</button>
							</div>
						</div>
						<div v-if="percent != null">
							{{ percent }}%
						</div>
					</form>
				</div>
			</div>
		</div>
	</div>
</span>
	`,
});
