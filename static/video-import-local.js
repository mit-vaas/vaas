Vue.component('video-import-local', {
	data: function() {
		return {
			path: '',
			optSymlink: false,
			optTranscode: true,
		};
	},
	props: ['series'],
	mounted: function() {
		$('#v-local-modal').on('shown.bs.modal', function() {
			$('#v-local-name').focus();
		});
	},
	methods: {
		click: function() {
			this.path = '';
			this.optSymlink = false;
			this.optTranscode = true;
			$('#v-local-modal').modal('show');
		},
		submit: function() {
			var params = {
				series_id: this.series.ID,
				path: this.path,
				symlink: this.optSymlink ? 'yes' : 'no',
				transcode: this.optTranscode ? 'yes' : 'no',
			};
			$.post('/import/local', params, function() {
				$('#v-local-modal').modal('hide');
				this.$emit('imported');
			}.bind(this));
		},
	},
	template: `
<span>
	<button type="button" class="btn btn-primary" v-on:click=click>Import from Local</button>
	<div class="modal" tabindex="-1" role="dialog" id="v-local-modal">
		<div class="modal-dialog" role="document">
			<div class="modal-content">
				<div class="modal-body">
					<form v-on:submit.prevent="submit">
						<div class="form-group row">
							<label class="col-sm-2 col-form-label">Path</label>
							<div class="col-sm-10">
								<input class="form-control" type="text" v-model="path" />
							</div>
						</div>
						<fieldset class="form-group">
							<div class="row">
								<legend class="col-form-label col-sm-2 pt-0">Options</legend>
								<div class="col-sm-10">
									<div class="form-check">
										<input class="form-check-input" type="checkbox" v-model="optSymlink" value="yes">
										<label class="form-check-label">
											Symlink
										</label>
									</div>
									<div class="form-check">
										<input class="form-check-input" type="checkbox" v-model="optTranscode" value="yes">
										<label class="form-check-label">
											Transcode
										</label>
									</div>
								</div>
							</div>
						</fieldset>
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
