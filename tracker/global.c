/* SPDX-License-Identifier: GPL-3.0-or-later */
#include "global.h"
#include <stdarg.h>

    /* Natural number types, used in hash functions and elsewhere: */
typedef unsigned long	Nat;
typedef unsigned short	Nat16;

	/* Routines to allocate and deallocate memory.
	 * emalloc() aborts if insufficient memory is available.
	 * logprintf() writes messages to logfile adt.log
	 * If tracemalloc is called with argument TRUE, the result of each
	 *	emalloc/efree call is written to the logfile.
	 */

#define	LOGNAME	"c:\\adt.log"
static FILE *logFile = NULL;
static Bool	tracing = FALSE;
static Nat	seqNum = 0;

static void error(char *mssg, char *fileName, int lineNumber)
{
	fprintf(stderr, "Memory handler error in %s (%d): %s\n",
		fileName, lineNumber, mssg);
	abort();
}

Generic emalloc(size_t size, char * fileName, int lineNumber)
{
	Generic	p = malloc(size == 0? 1 : size);

  if (tracing) {
	    logprintf("A %p %07ld %lu %s (%d)\n", 
		p, ++seqNum, (unsigned long) size, fileName, lineNumber);
  fflush(logFile);
  }
	if (p == NULL)
	    error("out of memory", fileName, lineNumber);
	return p;
}

Generic erealloc(Generic object, size_t size, char * fileName, int lineNumber)
{
	Generic	p = realloc(object, size);

	if (tracing) {
	    logprintf("R %p %07ld %s (%d)\n", object, ++seqNum, fileName, lineNumber);
	    logprintf("A %p %07ld %lu %s (%d) \n", 
		p, seqNum, (unsigned long) size, fileName, lineNumber, p);

	}
	if (p == NULL)
	    error("out of memory", fileName, lineNumber);
	return p;
}

void efree(Generic object, char * fileName, int lineNumber)
{
  if (tracing) {
	    logprintf("F %p %07ld %s (%d)\n", object, ++seqNum, fileName, lineNumber);
  fflush(logFile);
  }
	if (object == NULL)
	    error("cannot free NULL", fileName, lineNumber);
	free(object);
}

int logprintf(const char *fmt, ...)
{
	va_list	args;

	if (logFile == NULL)
	    if ((logFile = fopen(LOGNAME, "w")) == NULL)
		return EOF;
	va_start(args, fmt);
	return vfprintf(logFile, fmt, args);
}
	
void tracemalloc(Bool traceOn)
{
	tracing = traceOn;
/*  tracing = FALSE; */
}

char *format(const char *fmt, ...)
{
	static char buf[BUFSIZ];
	va_list	args;

	va_start(args, fmt);
	vsprintf(buf, fmt, args);
	return buf;
}
