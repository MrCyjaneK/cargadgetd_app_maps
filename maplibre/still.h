#ifndef CG_ML_STILL_H
#define CG_ML_STILL_H

#include <stdint.h>

/* One camera the painter asked for. pitch is degrees.
   style is an OSM Scout Mapbox style id, such as osmbright. */
typedef struct cg_ml_job {
	double lat, lon, zoom, bearing, pitch;
	int w, h;
	char style[48];
} cg_ml_job;

/* 0: *rgba is a straight tightly packed image the caller frees.
   1: style or tiles are still loading.
   2: EGL or the library cannot draw. */
int cg_ml_render(const cg_ml_job *job, uint8_t **rgba, int *w, int *h);
/* Zoom of the pixels last returned. Lower than the request when Scout
   had no tiles at the asked zoom. */
double cg_ml_drawn_zoom(void);
void cg_ml_pause(void);

/* Projects n points through the camera of the last still. 0 on success. */
int cg_ml_project(const double *lat, const double *lon, int n, double *x, double *y);

#endif
