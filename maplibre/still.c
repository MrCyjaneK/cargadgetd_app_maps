//go:build maplibrenative

#define _GNU_SOURCE

/* One MapLibre map for the process. The first Render starts this thread;
   plugin init never touches EGL. Callbacks stay in C so they do not enter
   Go while Codec2 is alive. Pause tears the map and its EGL context down. */

#include "still.h"

#include <maplibre_native_c.h>

#include <math.h>
#include <pthread.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

static pthread_mutex_t mu = PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t cv = PTHREAD_COND_INITIALIZER;
static pthread_t thread;
static int thread_started;
static int stop;
static int failed;
static int logged;

static cg_ml_job wanted;
static int have_wanted;
static cg_ml_job done_job;
static int have_image;
static uint8_t *image;
static int image_w, image_h;
static double drawn_zoom;
static mln_map_view view;
static int have_view;

static pthread_mutex_t wait_mu = PTHREAD_MUTEX_INITIALIZER;
static pthread_cond_t wait_cv = PTHREAD_COND_INITIALIZER;

typedef struct cg_slot {
	int done;
	int copy;
	mln_status status;
	uint8_t *pixels;
	int w, h;
	char message[240];
} cg_slot;

static cg_slot slot;

static mln_status cg_wait(void);

static void cg_log(const char *what, const mln_diagnostic *d) {
	const char *msg = (d != NULL && d->message[0] != '\0') ? d->message : "(no diagnostic)";
	fprintf(stderr, "maps: maplibre %s: %s\n", what, msg);
}

static void cg_log_slot(const char *what) {
	fprintf(stderr, "maps: maplibre %s: %s\n", what, slot.message[0] != '\0' ? slot.message : "(no diagnostic)");
}

static int cg_failed(const char *what, mln_status st, const mln_diagnostic *d) {
	if (st != MLN_STATUS_OK) {
		cg_log(what, d);
		return -1;
	}
	if (cg_wait() != MLN_STATUS_OK) {
		cg_log_slot(what);
		return -1;
	}
	return 0;
}

static mln_diagnostic cg_diag(void) {
	mln_diagnostic d;
	memset(&d, 0, sizeof d);
	d.size = sizeof d;
	return d;
}

static int cg_same(const cg_ml_job *a, const cg_ml_job *b) {
	return a->w == b->w && a->h == b->h && a->lat == b->lat && a->lon == b->lon &&
	       a->zoom == b->zoom && a->bearing == b->bearing && a->pitch == b->pitch &&
	       strcmp(a->style, b->style) == 0;
}

static void cg_copy_straight(uint8_t *dst, const uint8_t *src, uint32_t w, uint32_t h, uint32_t stride) {
	for (uint32_t y = 0; y < h; y++) {
		const uint8_t *row = src + (size_t)y * stride;
		uint8_t *out = dst + (size_t)y * w * 4;
		for (uint32_t x = 0; x < w; x++) {
			uint32_t r = row[x * 4 + 0];
			uint32_t g = row[x * 4 + 1];
			uint32_t b = row[x * 4 + 2];
			uint32_t a = row[x * 4 + 3];
			if (a == 0) {
				out[x * 4 + 0] = out[x * 4 + 1] = out[x * 4 + 2] = out[x * 4 + 3] = 0;
			} else if (a == 255) {
				out[x * 4 + 0] = (uint8_t)r;
				out[x * 4 + 1] = (uint8_t)g;
				out[x * 4 + 2] = (uint8_t)b;
				out[x * 4 + 3] = 255;
			} else {
				out[x * 4 + 0] = (uint8_t)((r * 255) / a);
				out[x * 4 + 1] = (uint8_t)((g * 255) / a);
				out[x * 4 + 2] = (uint8_t)((b * 255) / a);
				out[x * 4 + 3] = (uint8_t)a;
			}
		}
	}
}

static void cg_on_done(void *user, const mln_completion_result *result) {
	cg_slot *s = user;
	pthread_mutex_lock(&wait_mu);
	s->status = result->status;
	if (s->copy && result->status == MLN_STATUS_OK && result->value_count >= 1 && result->value != NULL) {
		const mln_texture_readback_result *rb = result->value;
		uint32_t w = rb->info.width;
		uint32_t h = rb->info.height;
		uint32_t stride = rb->info.stride;
		size_t n = (size_t)w * h * 4;
		uint8_t *px = NULL;
		if (w > 0 && h > 0 && stride >= w * 4 && rb->data.data != NULL && rb->data.size >= (size_t)stride * h) {
			px = malloc(n);
		}
		if (px == NULL) {
			s->status = MLN_STATUS_NATIVE_ERROR;
		} else {
			cg_copy_straight(px, rb->data.data, w, h, stride);
			s->pixels = px;
			s->w = (int)w;
			s->h = (int)h;
		}
	}
	s->message[0] = '\0';
	if (result->diagnostic.data != NULL && result->diagnostic.size > 0) {
		size_t n = result->diagnostic.size;
		if (n > sizeof s->message - 1) {
			n = sizeof s->message - 1;
		}
		memcpy(s->message, result->diagnostic.data, n);
		s->message[n] = '\0';
	}
	s->done = 1;
	pthread_cond_signal(&wait_cv);
	pthread_mutex_unlock(&wait_mu);
}

static mln_completion cg_completion(int copy) {
	slot.done = 0;
	slot.copy = copy;
	slot.status = MLN_STATUS_NATIVE_ERROR;
	slot.pixels = NULL;
	slot.w = 0;
	slot.h = 0;
	slot.message[0] = '\0';
	mln_completion c;
	memset(&c, 0, sizeof c);
	c.size = sizeof c;
	c.callback = cg_on_done;
	c.user_data = &slot;
	return c;
}

static mln_status cg_wait(void) {
	pthread_mutex_lock(&wait_mu);
	while (!slot.done) {
		pthread_cond_wait(&wait_cv, &wait_mu);
	}
	mln_status st = slot.status;
	pthread_mutex_unlock(&wait_mu);
	return st;
}

static void cg_block_sigurg(void) {
	sigset_t set;
	sigemptyset(&set);
	sigaddset(&set, SIGURG);
	pthread_sigmask(SIG_BLOCK, &set, NULL);
}

static mln_runtime runtime = MLN_HANDLE_NULL;
static mln_map map = MLN_HANDLE_NULL;
static mln_graphics_device device = MLN_HANDLE_NULL;
static mln_render_session session = MLN_HANDLE_NULL;
static int map_w, map_h;
static int style_set;
static char style_id[48];
/* Closest zoom Scout could draw near ceil_lat/lon. A 418 means that
   place is not in the download, so a closer camera is asked again here. */
static int have_ceiling;
static double ceiling_zoom;
static double ceil_lat, ceil_lon;
static const double kCeilingM = 250000.0;

static void cg_close(void) {
	mln_diagnostic d = cg_diag();
	if (session != MLN_HANDLE_NULL) {
		mln_completion c = cg_completion(0);
		if (mln_render_session_dispose(session, &c, &d) == MLN_STATUS_OK) {
			cg_wait();
		}
		session = MLN_HANDLE_NULL;
	}
	if (map != MLN_HANDLE_NULL) {
		mln_completion c = cg_completion(0);
		if (mln_map_release(map, &c, &d) == MLN_STATUS_OK) {
			cg_wait();
		}
		map = MLN_HANDLE_NULL;
	}
	if (runtime != MLN_HANDLE_NULL) {
		mln_completion c = cg_completion(0);
		if (mln_runtime_release(runtime, &c, &d) == MLN_STATUS_OK) {
			cg_wait();
		}
		runtime = MLN_HANDLE_NULL;
	}
	if (device != MLN_HANDLE_NULL) {
		mln_graphics_device_release(device);
		device = MLN_HANDLE_NULL;
	}
	style_set = 0;
	style_id[0] = '\0';
	have_ceiling = 0;
	map_w = 0;
	map_h = 0;
	pthread_mutex_lock(&mu);
	have_view = 0;
	pthread_mutex_unlock(&mu);
}

static int cg_open(int w, int h) {
	mln_diagnostic d = cg_diag();
	mln_runtime_options ro;
	memset(&ro, 0, sizeof ro);
	ro.size = sizeof ro;
	runtime = MLN_HANDLE_NULL;
	if (mln_runtime_create(&ro, &runtime, &d) != MLN_STATUS_OK) {
		cg_log("runtime", &d);
		return -1;
	}

	mln_map_options opt;
	memset(&opt, 0, sizeof opt);
	opt.size = sizeof opt;
	opt.initial_extent.width = (uint32_t)w;
	opt.initial_extent.height = (uint32_t)h;
	opt.initial_extent.scale_factor = 1.0;
	opt.map_mode = MLN_MAP_MODE_STATIC;
	map = MLN_HANDLE_NULL;
	mln_completion created = cg_completion(0);
	if (cg_failed("map", mln_runtime_create_map(runtime, &opt, &map, &created, &d), &d) != 0) {
		return -1;
	}

	mln_graphics_device_options go;
	memset(&go, 0, sizeof go);
	go.size = sizeof go;
	go.fields = MLN_GRAPHICS_DEVICE_OPTION_SINGLE_CONTEXT;
	go.single_context = true;
	device = MLN_HANDLE_NULL;
	if (mln_graphics_device_create(&go, &device, &d) != MLN_STATUS_OK) {
		cg_log("egl context", &d);
		return -1;
	}
	fprintf(stderr, "maps: maplibre egl context created on the still renderer, after plugin load\n");

	mln_graphics_device_texture_descriptor desc;
	memset(&desc, 0, sizeof desc);
	desc.size = sizeof desc;
	desc.extent.width = (uint32_t)w;
	desc.extent.height = (uint32_t)h;
	desc.extent.scale_factor = 1.0;
	mln_render_session_attach_options ao;
	memset(&ao, 0, sizeof ao);
	ao.size = sizeof ao;
	ao.driver = MLN_RENDER_DRIVER_CORE_WORKER;
	session = MLN_HANDLE_NULL;
	mln_completion attached = cg_completion(0);
	if (cg_failed("texture", mln_map_attach_graphics_device_texture(map, device, &desc, &ao, &session, &attached, &d), &d) != 0) {
		return -1;
	}
	map_w = w;
	map_h = h;
	return 0;
}

static int cg_camera(const cg_ml_job *job) {
	mln_camera_update update;
	memset(&update, 0, sizeof update);
	update.size = sizeof update;
	update.mode = MLN_CAMERA_UPDATE_MODE_JUMP;
	update.camera.size = sizeof update.camera;
	update.camera.fields = MLN_CAMERA_OPTION_CENTER | MLN_CAMERA_OPTION_ZOOM | MLN_CAMERA_OPTION_BEARING | MLN_CAMERA_OPTION_PITCH;
	update.camera.center.latitude = job->lat;
	update.camera.center.longitude = job->lon;
	update.camera.zoom = job->zoom;
	update.camera.bearing = job->bearing;
	update.camera.pitch = job->pitch;
	mln_diagnostic d = cg_diag();
	mln_completion c = cg_completion(0);
	return cg_failed("camera", mln_map_update_camera(map, &update, &c, &d), &d);
}

static int cg_style_ok(const char *id) {
	size_t n;
	if (id == NULL) {
		return 0;
	}
	n = strlen(id);
	if (n == 0 || n >= sizeof style_id) {
		return 0;
	}
	for (size_t i = 0; i < n; i++) {
		char c = id[i];
		if (!((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_')) {
			return 0;
		}
	}
	return 1;
}

static int cg_style(const cg_ml_job *job) {
	const char *id = cg_style_ok(job->style) ? job->style : "osmbright";
	if (style_set && strcmp(style_id, id) == 0) {
		return 0;
	}
	char url[128];
	snprintf(url, sizeof url, "http://127.0.0.1:8553/v1/mbgl/style?style=%s", id);
	mln_diagnostic d = cg_diag();
	mln_completion c = cg_completion(0);
	if (cg_failed("style", mln_map_set_style_url(map, mln_buffer_view_from_cstr(url), &c, &d), &d) != 0) {
		return -1;
	}
	snprintf(style_id, sizeof style_id, "%s", id);
	style_set = 1;
	return 0;
}

static double cg_meters(double lat1, double lon1, double lat2, double lon2) {
	const double r = 6371000.0;
	double p1 = lat1 * 3.141592653589793 / 180.0;
	double p2 = lat2 * 3.141592653589793 / 180.0;
	double dp = (lat2 - lat1) * 3.141592653589793 / 180.0;
	double dl = (lon2 - lon1) * 3.141592653589793 / 180.0;
	double sdp = sin(dp / 2.0);
	double sdl = sin(dl / 2.0);
	double a = sdp * sdp + cos(p1) * cos(p2) * sdl * sdl;
	return 2.0 * r * atan2(sqrt(a), sqrt(1.0 - a));
}

static int cg_ceiling_here(const cg_ml_job *job) {
	return have_ceiling && cg_meters(job->lat, job->lon, ceil_lat, ceil_lon) <= kCeilingM;
}

static void cg_remember_ceiling(double zoom, double lat, double lon) {
	int fresh = !have_ceiling || zoom < ceiling_zoom - 0.01 || cg_meters(lat, lon, ceil_lat, ceil_lon) > kCeilingM;
	have_ceiling = 1;
	ceiling_zoom = zoom;
	ceil_lat = lat;
	ceil_lon = lon;
	if (fresh) {
		fprintf(stderr, "maps: zoom is not served (HTTP 418); staying at %.0f\n", zoom);
	}
}

/* drawn is the camera that produced the pixels. done is the camera the
   painter asked for, so a backed-off zoom still answers that request. */
static int cg_still(const cg_ml_job *drawn, const cg_ml_job *done) {
	mln_diagnostic d = cg_diag();
	mln_completion still = cg_completion(0);
	if (cg_failed("still", mln_map_request_still_image(map, &still, &d), &d) != 0) {
		return -1;
	}
	mln_completion read = cg_completion(1);
	if (cg_failed("readback", mln_render_session_read_texture(session, &read, &d), &d) != 0 || slot.pixels == NULL) {
		free(slot.pixels);
		slot.pixels = NULL;
		return -1;
	}
	if (slot.w != drawn->w || slot.h != drawn->h) {
		free(slot.pixels);
		slot.pixels = NULL;
		fprintf(stderr, "maps: maplibre readback %dx%d, wanted %dx%d\n", slot.w, slot.h, drawn->w, drawn->h);
		return -1;
	}
	mln_map_snapshot *snap = calloc(1, sizeof *snap);
	int got_view = 0;
	mln_map_view snapped;
	memset(&snapped, 0, sizeof snapped);
	if (snap != NULL) {
		snap->size = sizeof *snap;
		mln_diagnostic sd = cg_diag();
		if (mln_map_get_snapshot(map, snap, &sd) == MLN_STATUS_OK) {
			snapped = snap->view;
			got_view = 1;
		}
		free(snap);
	}
	pthread_mutex_lock(&mu);
	free(image);
	image = slot.pixels;
	slot.pixels = NULL;
	image_w = slot.w;
	image_h = slot.h;
	done_job = *done;
	drawn_zoom = drawn->zoom;
	have_image = 1;
	if (got_view) {
		view = snapped;
		have_view = 1;
	}
	pthread_mutex_unlock(&mu);
	return 0;
}

static int cg_is_418(void) {
	return slot.message[0] != '\0' && strstr(slot.message, "418") != NULL;
}

/* A missing download is HTTP 418. Draw the closest zoom that still has
   tiles, and keep using it while the view stays on that place. */
static int cg_view(const cg_ml_job *want) {
	cg_ml_job draw;
	int dropped = 0;
	int step;
	if (want == NULL) {
		return -1;
	}
	if (have_ceiling && cg_meters(want->lat, want->lon, ceil_lat, ceil_lon) > kCeilingM) {
		have_ceiling = 0;
	}
	draw = *want;
	if (cg_ceiling_here(want) && draw.zoom > ceiling_zoom) {
		draw.zoom = ceiling_zoom;
		dropped = 1;
	}
	for (step = 0; step < 20; step++) {
		if (cg_camera(&draw) != 0 || cg_style(&draw) != 0) {
			return -1;
		}
		if (cg_still(&draw, want) == 0) {
			if (dropped) {
				cg_remember_ceiling(draw.zoom, want->lat, want->lon);
			}
			return 0;
		}
		if (!cg_is_418()) {
			return -1;
		}
		{
			double next = floor(draw.zoom) - 1.0;
			if (next < 0.0 || !(next < draw.zoom - 0.01)) {
				return -1;
			}
			draw.zoom = next;
			dropped = 1;
		}
	}
	return -1;
}

int cg_ml_project(const double *lat, const double *lon, int n, double *x, double *y) {
	if (n <= 0 || lat == NULL || lon == NULL || x == NULL || y == NULL) {
		return -1;
	}
	pthread_mutex_lock(&mu);
	if (!have_view) {
		pthread_mutex_unlock(&mu);
		return -1;
	}
	mln_map_view v = view;
	pthread_mutex_unlock(&mu);

	mln_lat_lng *coords = calloc((size_t)n, sizeof *coords);
	mln_screen_point *pts = calloc((size_t)n, sizeof *pts);
	if (coords == NULL || pts == NULL) {
		free(coords);
		free(pts);
		return -1;
	}
	for (int i = 0; i < n; i++) {
		coords[i].latitude = lat[i];
		coords[i].longitude = lon[i];
	}
	mln_map_projection proj = MLN_HANDLE_NULL;
	mln_diagnostic d = cg_diag();
	mln_status st = mln_map_projection_create(&v, &proj, &d);
	if (st != MLN_STATUS_OK) {
		free(coords);
		free(pts);
		return -1;
	}
	st = mln_map_projection_pixels_for_lat_lngs(proj, coords, (size_t)n, pts, &d);
	mln_map_projection_release(proj);
	if (st != MLN_STATUS_OK) {
		free(coords);
		free(pts);
		return -1;
	}
	for (int i = 0; i < n; i++) {
		x[i] = pts[i].x;
		y[i] = pts[i].y;
	}
	free(coords);
	free(pts);
	return 0;
}

static void *cg_main(void *unused) {
	(void)unused;
	cg_block_sigurg();
	for (;;) {
		pthread_mutex_lock(&mu);
		while (!stop && (!have_wanted || (have_image && cg_same(&wanted, &done_job)))) {
			pthread_cond_wait(&cv, &mu);
		}
		if (stop) {
			pthread_mutex_unlock(&mu);
			break;
		}
		cg_ml_job job = wanted;
		pthread_mutex_unlock(&mu);

		if (map == MLN_HANDLE_NULL || map_w != job.w || map_h != job.h) {
			cg_close();
			if (cg_open(job.w, job.h) != 0) {
				cg_close();
				pthread_mutex_lock(&mu);
				failed = 1;
				if (!logged) {
					logged = 1;
					fprintf(stderr, "maps: maplibre still renderer has no image\n");
				}
				pthread_mutex_unlock(&mu);
				break;
			}
		}
		if (cg_view(&job) != 0) {
			/* Scout or the style is still coming up. Ask again on the next paint. */
			usleep(200000);
		}
	}
	cg_close();
	return NULL;
}

int cg_ml_render(const cg_ml_job *job, uint8_t **rgba, int *w, int *h) {
	if (job == NULL || rgba == NULL || w == NULL || h == NULL || job->w <= 0 || job->h <= 0) {
		return 2;
	}
	*rgba = NULL;
	pthread_mutex_lock(&mu);
	if (failed) {
		pthread_mutex_unlock(&mu);
		return 2;
	}
	wanted = *job;
	have_wanted = 1;
	if (!thread_started) {
		stop = 0;
		if (pthread_create(&thread, NULL, cg_main, NULL) != 0) {
			failed = 1;
			pthread_mutex_unlock(&mu);
			fprintf(stderr, "maps: maplibre still thread failed\n");
			return 2;
		}
		thread_started = 1;
	} else {
		pthread_cond_signal(&cv);
	}
	if (!(have_image && cg_same(&done_job, job))) {
		pthread_mutex_unlock(&mu);
		return 1;
	}
	size_t n = (size_t)image_w * (size_t)image_h * 4;
	uint8_t *copy = malloc(n);
	if (copy == NULL) {
		pthread_mutex_unlock(&mu);
		return 1;
	}
	memcpy(copy, image, n);
	*rgba = copy;
	*w = image_w;
	*h = image_h;
	pthread_mutex_unlock(&mu);
	return 0;
}

double cg_ml_drawn_zoom(void) {
	double z;
	pthread_mutex_lock(&mu);
	z = drawn_zoom;
	pthread_mutex_unlock(&mu);
	return z;
}

void cg_ml_pause(void) {
	pthread_mutex_lock(&mu);
	stop = 1;
	pthread_cond_broadcast(&cv);
	pthread_t t = thread;
	int started = thread_started;
	thread_started = 0;
	pthread_mutex_unlock(&mu);
	if (started) {
		pthread_join(t, NULL);
	}
	pthread_mutex_lock(&mu);
	stop = 0;
	failed = 0;
	logged = 0;
	have_wanted = 0;
	have_image = 0;
	have_view = 0;
	free(image);
	image = NULL;
	pthread_mutex_unlock(&mu);
}
