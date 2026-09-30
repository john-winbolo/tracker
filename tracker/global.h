/* SPDX-License-Identifier: GPL-3.0-or-later */
#ifndef _GLOBAL_H  /* Double inclusion protection */
#define _GLOBAL_H
#define _UNICODE
#define UNICODE

/* Standard Includes */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>   /* Fixed-width integer types for x64 compatibility */


/* Byte type def */
typedef unsigned char  BYTE;

/* Boolean type */
#undef TRUE
#define TRUE 1
#undef FALSE
#define FALSE 0
typedef BYTE bool;
typedef BYTE Bool;

/* WORD data type */
typedef unsigned short WORD;


/* Strings of 36 charectors */
#define MAP_STR_SIZE 36


/* The type compatible with all other pointers types;
 * used where the corresponding type is indeterminate
 */
typedef void *Generic;

/* Fixed Strings, STRINGSIZE and SHORTSTRINGSIZE set buffer length.*/
#ifndef FILENAME_MAX  /* Usually somwhere in stdio.h */
#define FILENAME_MAX 256
#endif

#define STRINGSIZE FILENAME_MAX
#define SHORTSTRINGSIZE 32
#define	IP_SIZE	4
typedef char *StringRef;
typedef char StringBuf[STRINGSIZE];
typedef char ShortStringBuf[SHORTSTRINGSIZE];


/***************************** Assertions ***************************/

    /* Assert() is an alternative to assert() that prints the
     * user-supplied message in addition to textual position of call.
     */
#if	defined(NDEBUG)			/* ignore assertion as assert() */
#define	Assert(cond, mssg)	((void) FALSE)
#else
#define	Assert(cond, mssg)	((void) ((cond)? FALSE : (fprintf(stderr, \
				"Assertion failed in %s (%d): %s\n",      \
				__FILE__, __LINE__, (mssg)), abort())))
#endif
#define	Defined(obj)	Assert((obj) != NULL, "undefined object")

    /* Format strings to pass to Assert. Result is overwritten each call */
char *format(const char *fmt, ...);

/***************************** Memory Allocation  ***************************/

    /* emalloc(), efree(), logprintf() and tracemalloc() are defined in adt.c.
     * They are normally accessed using the New, Dispose and Copy macros.
     * emalloc() aborts if insufficient memory is available.
     * logprintf() writes messages to logfile adt.log
     * If tracemalloc is called with argument TRUE, the result of each
     * emalloc/efree call is written to the logfile.
     */
Generic emalloc(size_t size, char *fileName, int lineNumber);
Generic erealloc(Generic obj, size_t size, char *fileName, int lineNumber);
void efree(Generic object, char *fileName, int lineNumber);
int logprintf(const char *fmt, ...);
void tracemalloc(Bool traceOn);

#define	New(p)			((p) = emalloc(sizeof(*(p)), __FILE__, __LINE__))
#define	Resize(p, newSize)	((p) = erealloc((p), (newSize), __FILE__, __LINE__))
#define	Dispose(p)	(efree(p, __FILE__, __LINE__), p = NULL)
#define	Copy(buf, size)	memcpy(emalloc(size, __FILE__, __LINE__), (buf), (size))
#define	stralloc(s)	strcpy(emalloc(strlen(s)+1, __FILE__, __LINE__), (s))
    /* stralloc is a safe version of strdup, found in some C libraries */

/***************************** Macro Extensions ***************************/

    /* The mechanism used in most adt *.h files to generate identifiers
     * unique to the specific ADT is to concatenate the user-defined
     * prefix and a standard suffix. The following is ANSI-C specific:
     */
#define	_glue(a,b)		a##b
#define	_concat(pre,suf)	_glue(pre,suf)

    /* A synonym for the macro argument to string literal operator: */
#define	STRING(name)	#name

    /* These allow macros to consist of a block instead of a single expression.
     * usage is
     *	#define thing(a,b)	OPEN_BLOCK	\
     *		int	foo, bar;		\
     *		foo = a;			\
     *		   ...				\
     *	CLOSE_BLOCK
     */
#define	OPEN_BLOCK	do {
#define	CLOSE_BLOCK	} while(FALSE)


/* Game types  */
typedef enum {
  gameOpen = 1,
  gameTournament,
  gameStrictTournament
} gameType;


typedef enum {
  aiNone,
  aiYes,
  aiYesAdvantage,
  aiYesFull
} aiType;


#define MAX_CONNECTIONS 128


#ifndef _WIN32
/* Linux doesn't have SOCKET type, define it */
typedef unsigned int SOCKET;
#include <netinet/in.h>  /* For struct in_addr */
#endif

#define NEWLINE_CHAR '\n'
#define EMPTY_CHAR '\0'

#ifdef _WIN32
typedef unsigned int socklen_t;
#define	fd_set	FD_SET
#endif

#endif /* _GLOBAL_H */ 

