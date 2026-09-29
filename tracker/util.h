/*********************************************************
*Name:          Util
*Filename:      util.h
*Author:        John Morrison
*Creation Date: 00/10/01
*Last Modified: 00/10/01
*Purpose:
*  Provides misc functions
*********************************************************/

#ifndef _UTIL_H
#define _UTIL_H

#ifdef _WIN32
#define	sleep(X)	sleep(X)
#else
#define	sleep(X)	usleep(X)
#endif


/*********************************************************
*NAME:          utilMakeBoundSocket
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: 00/10/01
*PURPOSE:
* Makes a bound socket and returns it. Returns 
* SOCKET_ERROR on error
*
*ARGUMENTS:
*  tcp       - TRUE if a TCP socket, else false
*  listening - TRUE if the socket is to listen for 
*              connections
*  blocking  - TRUE if this should be a blocking socket
*  port      - Port to bind to
*********************************************************/
SOCKET utilMakeBoundSocket(bool tcp, bool listening, bool blocking, unsigned short port);

/*********************************************************
*NAME:          utilReverseLookup
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: 00/10/01
*PURPOSE:
* Performs a reverse lookup on an IP
*
*ARGUMENTS:
*  pack - Source ip packet
*  dest - Destination space
*********************************************************/
bool utilReverseLookup(struct in_addr *pack, char *dest);

/*********************************************************
*NAME:          utilPtoCString
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: 00/10/01
*PURPOSE:
* Convert Bolo's network pascal string to C strings
*
*ARGUMENTS:
*  src  - Source string
*  dest - Destination string 
*********************************************************/
void utilPtoCString(char *src, char *dest);

#endif /* _UTIL_H */
