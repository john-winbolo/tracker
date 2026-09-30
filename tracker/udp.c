/* SPDX-License-Identifier: GPL-3.0-or-later */
/*********************************************************
*Name:          udp
*Filename:      udp.c
*Author:        John Morrison
*Creation Date: 00/10/01
*LAST MODIFIED: 00/10/01
*Purpose:
*  Responsable for handling incoming UDP game update
*  packets
*********************************************************/


#ifdef _WIN32
/* Windows winsock */
#include <winsock2.h>
#include <windows.h>
#include <memory.h>
#include <time.h>

#else
/* Liunux Winsock plus some useful macros */
#include        <string.h>
#include        <netinet/in.h>
#include        <arpa/inet.h>   /* for inet_ntoa() */
#include        <pthread.h>
#include        <unistd.h>      /* for close() */
#define closesocket(X) close(X)
typedef struct hostent  HOSTENT;
#define SD_BOTH 2
#define INVALID_SOCKET -1
#define SOCKET_ERROR -1

#endif

#include <stdio.h>
#include "global.h"
#include "util.h"
#include "currentgames.h"
#include "bans.h"
#include "udp.h"
#include "stats.h" /* for udpMaxThreads */

/* Define global variables */
int udpMaxThreads = 0;
int udpNumThreads = 0;

#define	UDP_MAX_THREADS	10

SOCKET udpSocket = SOCKET_ERROR;
udpThreadInfoStruct *udpThreadInfo[UDP_MAX_THREADS];
FILE *INFOPACKET_LOG;

/* Cross-platform ctime_r equivalent */
static void thread_safe_ctime(const time_t *timep, char *buf, size_t buflen) {
#ifdef _WIN32
    /* Windows: use ctime_s which is thread-safe */
    ctime_s(buf, buflen, timep);
#else
    /* Linux: use ctime_r */
    ctime_r(timep, buf);
#endif
}

/*********************************************************
*NAME:          udpCreate
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: 00/10/01
*PURPOSE:
* Sets up the udp module and returns the socket handle
*
*ARGUMENTS:
*  port - Port to bind to
*********************************************************/
SOCKET udpCreate(unsigned short port) {
  udpSocket = utilMakeBoundSocket(FALSE, FALSE, FALSE, port);
  udpNumThreads = 0;
  INFOPACKET_LOG = fopen("games.log", "w+");

  /* Debug: Verify socket creation */
  if (udpSocket == INVALID_SOCKET) {
    fprintf(stderr, "UDP: ERROR - Failed to create UDP socket on port %d\n", port);
  } else {
    fprintf(stderr, "UDP: Socket created successfully, fd=%d, port=%d\n", (int)udpSocket, port);
  }

  return udpSocket;
}

/*********************************************************
*NAME:          udpDestroy
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: 00/10/01
*PURPOSE:
* Cleansup after the udp module
*
*ARGUMENTS:
*
*********************************************************/
void udpDestroy() {
	int i;
  if (udpSocket != SOCKET_ERROR) {
    shutdown(udpSocket, SD_BOTH);
    closesocket(udpSocket);
    udpSocket = SOCKET_ERROR;

    for (i = 0; i < udpNumThreads; i++) {
#ifdef _WIN32
	    DeleteCriticalSection(&(udpThreadInfo[i]->mutex));
#else
	    pthread_mutex_destroy(&(udpThreadInfo[i]->mutex));
	    pthread_cond_destroy(&(udpThreadInfo[i]->signal));
#endif
	    free(udpThreadInfo[i]);
    }
  }
}

/*********************************************************
*NAME:          udpSendPunchAck
*PURPOSE:
* Sends a PACKET_PUNCH_REQUEST_ACK back to the joiner.
* 9-byte wire format, all multi-byte fields big-endian:
*   0  'W'
*   1  'B'
*   2  PACKET_PUNCH_REQUEST_ACK
*   3  reserved (0)
*   4  sequence (u32 BE, unused -- 0)
*   8  status byte (PUNCH_ACK_*)
*
*ARGUMENTS:
*  joinerIp   - reflexive IP (network byte order)
*  joinerPort - reflexive port (host byte order)
*  status     - PUNCH_ACK_*
*********************************************************/
static void udpSendPunchAck(uint32_t joinerIp, uint16_t joinerPort, uint8_t status) {
	uint8_t buf[9];
	struct sockaddr_in dest;

	buf[0] = 'W';
	buf[1] = 'B';
	buf[2] = (uint8_t)PACKET_PUNCH_REQUEST_ACK;
	buf[3] = 0;
	memset(buf + 4, 0, 4);          /* sequence -- unused */
	buf[8] = status;

	memset(&dest, 0, sizeof(dest));
	dest.sin_family = AF_INET;
	dest.sin_addr.s_addr = joinerIp;
	dest.sin_port = htons(joinerPort);
	sendto(udpSocket, (const char *)buf, sizeof(buf), 0,
	       (const struct sockaddr *)&dest, sizeof(dest));
}

/*********************************************************
*NAME:          udpSendPunchNotify
*PURPOSE:
* Sends PACKET_PUNCH_NOTIFY to the host's live NAT mapping
* (sourceIp:sourcePort, NOT the registered ip:port -- the
* registered address may be unroutable from the tracker,
* which is the whole reason hole-punching exists).
* 14-byte wire format, all multi-byte fields big-endian:
*   0  'W'
*   1  'B'
*   2  PACKET_PUNCH_NOTIFY
*   3  reserved (0)
*   4  sequence (u32 BE, unused -- 0)
*   8  joiner reflexive IP (4 raw bytes, network order)
*  12  joiner reflexive port (u16 BE)
*
*ARGUMENTS:
*  hostSourceIp   - host's live NAT IP (network byte order)
*  hostSourcePort - host's live NAT port (host byte order)
*  joinerIp       - joiner reflexive IP (network byte order)
*  joinerPort     - joiner reflexive port (host byte order)
*********************************************************/
static void udpSendPunchNotify(uint32_t hostSourceIp, uint16_t hostSourcePort,
                               uint32_t joinerIp, uint16_t joinerPort) {
	uint8_t buf[14];
	struct sockaddr_in dest;

	buf[0] = 'W';
	buf[1] = 'B';
	buf[2] = (uint8_t)PACKET_PUNCH_NOTIFY;
	buf[3] = 0;
	memset(buf + 4, 0, 4);          /* sequence -- unused */
	memcpy(buf + 8, &joinerIp, 4);  /* already network order */
	buf[12] = (uint8_t)(joinerPort >> 8);
	buf[13] = (uint8_t)(joinerPort & 0xFF);

	memset(&dest, 0, sizeof(dest));
	dest.sin_family = AF_INET;
	dest.sin_addr.s_addr = hostSourceIp;
	dest.sin_port = htons(hostSourcePort);
	sendto(udpSocket, (const char *)buf, sizeof(buf), 0,
	       (const struct sockaddr *)&dest, sizeof(dest));
}

/*********************************************************
*NAME:          udpSendPunchProbeReply
*PURPOSE:
* Sends PACKET_PUNCH_PROBE_REPLY back to the requester
* echoing the reflexive address we observed via recvfrom.
* Reply is sent on the same udpSocket to the same source
* the request arrived from -- the reply traversing the
* same NAT mapping the request opened is what proves
* bidirectional reachability. Stateless; no currentGames
* lookup, no locking.
* 14-byte wire format, all multi-byte fields big-endian:
*   0  'W'
*   1  'B'
*   2  PACKET_PUNCH_PROBE_REPLY
*   3  reserved (0)
*   4  sequence (u32 BE, unused -- 0)
*   8  reflexive IP (4 raw bytes, network order)
*  12  reflexive port (u16 BE)
*
*ARGUMENTS:
*  reflexiveIp   - source IP from recvfrom (network byte order)
*  reflexivePort - source port from recvfrom (host byte order)
*********************************************************/
static void udpSendPunchProbeReply(uint32_t reflexiveIp, uint16_t reflexivePort) {
	uint8_t buf[14];
	struct sockaddr_in dest;

	buf[0] = 'W';
	buf[1] = 'B';
	buf[2] = (uint8_t)PACKET_PUNCH_PROBE_REPLY;
	buf[3] = 0;
	memset(buf + 4, 0, 4);             /* sequence -- unused */
	memcpy(buf + 8, &reflexiveIp, 4);  /* already network order */
	buf[12] = (uint8_t)(reflexivePort >> 8);
	buf[13] = (uint8_t)(reflexivePort & 0xFF);

	memset(&dest, 0, sizeof(dest));
	dest.sin_family = AF_INET;
	dest.sin_addr.s_addr = reflexiveIp;
	dest.sin_port = htons(reflexivePort);
	sendto(udpSocket, (const char *)buf, sizeof(buf), 0,
	       (const struct sockaddr *)&dest, sizeof(dest));
}

/*********************************************************
*NAME:          udpHandlePunchRequest
*PURPOSE:
* Joiner asked us to nudge a registered host. Look up the
* host by (registered ip, registered port), snapshot its
* live NAT mapping under the games lock, then drop the
* lock before doing any sendto -- we don't want to hold
* the games lock across blocking I/O.
*
*ARGUMENTS:
*  cg         - the currentGames structure
*  joinerIp   - joiner reflexive IP (network byte order)
*  joinerPort - joiner reflexive port (host byte order)
*  targetIp   - host's registered IP (network byte order)
*  targetPort - host's registered port (host byte order)
*********************************************************/
static void udpHandlePunchRequest(currentGames *cg,
                                  uint32_t joinerIp, uint16_t joinerPort,
                                  uint32_t targetIp, uint16_t targetPort) {
	currentGames q;
	uint8_t  ackStatus;
	uint32_t hostSourceIp = 0;
	uint16_t hostSourcePort = 0;

	currentGamesLock();
	q = *cg;
	while (NonEmpty(q)) {
		if (memcmp(q->ip, &targetIp, IP_SIZE) == 0 && q->port == targetPort) {
			break;
		}
		q = q->next;
	}
	if (IsEmpty(q)) {
		ackStatus = PUNCH_ACK_HOST_NOT_FOUND;
	} else if (q->sourcePort == 0) {
		/* Host registered via INFO_PACKET but we've never seen a
		 * keepalive from it, so we have no live NAT mapping to push
		 * through. */
		ackStatus = PUNCH_ACK_HOST_UNREACHABLE;
	} else {
		ackStatus = PUNCH_ACK_OK;
		memcpy(&hostSourceIp, q->sourceIp, IP_SIZE);
		hostSourcePort = q->sourcePort;
	}
	currentGamesUnlock();

	udpSendPunchAck(joinerIp, joinerPort, ackStatus);
	if (ackStatus == PUNCH_ACK_OK) {
		udpSendPunchNotify(hostSourceIp, hostSourcePort,
		                   joinerIp, joinerPort);
	}
}

/*********************************************************
*NAME:          udpRead
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: 00/10/01
*PURPOSE:
* Checks for incoming game data packets
*
*ARGUMENTS:
*  cg - Pointer to current games structure
*********************************************************/
void udpRead(currentGames *cg) {
  int len;
  char buff[FILENAME_MAX];
  int szlast;
  struct sockaddr_in last;

  szlast = sizeof(last);

  fprintf(stderr, "UDP: udpRead() called, socket=%d\n", (int)udpSocket);
  fflush(stderr);
  
  len = recvfrom(udpSocket, buff, sizeof(buff), 0, (struct sockaddr *) &last, (socklen_t *) &szlast);
  
  /* Debug: Log recvfrom result */
  if (len < 0) {
#ifdef _WIN32
    int err = WSAGetLastError();
    fprintf(stderr, "UDP: recvfrom returned %d, error=%d (WSAEWOULDBLOCK=%d)\n", len, err, WSAEWOULDBLOCK);
#else
    fprintf(stderr, "UDP: recvfrom returned %d, errno=%d (%s), EAGAIN=%d\n", len, errno, strerror(errno), EAGAIN);
#endif
    fprintf(stderr, "UDP: done\n");
    return;
  }
  
  fprintf(stderr, "UDP: recvfrom returned %d bytes\n", len);
  fflush(stderr);
  
  while (len > 0) {
    fprintf(stderr,"UDP: Reading from udp socket ");
    fflush(stderr);

    /* WinBolo NAT keepalive: 4-byte sentinel sent by hosted servers to
     * keep their tracker NAT mapping alive between heavier registration
     * updates. No reply needed -- receipt itself refreshes the OS-level
     * conntrack mapping. We also refresh the matching currentGames
     * entry's source IP/port so PUNCH_NOTIFY targets the freshest
     * mapping. The 4-byte form is the original keepalive shape. */
    if (len == 4 &&
        buff[0] == 'W' && buff[1] == 'B' &&
        buff[2] == 'K' && buff[3] == 'A') {
      fprintf(stderr, "[wbka]\n");
      currentGamesRefreshSource(cg, (unsigned long)last.sin_addr.s_addr, ntohs(last.sin_port));
    } else if (len == 8 &&
               buff[0] == 'W' && buff[1] == 'B' &&
               buff[2] == 'K' && buff[3] == 'A') {
      /* 8-byte WBKA: bytes 4-7 carry a big-endian uint32_t game token
       * (the host's INFO_PACKET.gameid.start_time). Disambiguates the
       * case where one NAT IP hosts more than one WinBolo game. */
      uint32_t startTime;
      startTime = ((uint32_t)(uint8_t)buff[4] << 24) |
                  ((uint32_t)(uint8_t)buff[5] << 16) |
                  ((uint32_t)(uint8_t)buff[6] <<  8) |
                  ((uint32_t)(uint8_t)buff[7]);
      fprintf(stderr, "[wbka8]\n");
      currentGamesRefreshSourceExact(cg,
                                     (unsigned long)last.sin_addr.s_addr,
                                     ntohs(last.sin_port),
                                     (unsigned long)startTime);
    } else if (len == 8 &&
               buff[0] == 'W' && buff[1] == 'B' &&
               (uint8_t)buff[2] == PACKET_PUNCH_PROBE_REQUEST) {
      /* STUN-style host self-probe: stateless, simply reply to the
       * recvfrom source with the reflexive IP:port we observed. The
       * 4-byte 'WBKA' branches above match first so a keepalive can't
       * land here. */
      fprintf(stderr, "[probe]\n");
      udpSendPunchProbeReply((uint32_t)last.sin_addr.s_addr,
                             ntohs(last.sin_port));
    } else if (len == 14 &&
               buff[0] == 'W' && buff[1] == 'B' &&
               (uint8_t)buff[2] == PACKET_PUNCH_REQUEST) {
      /* Phase 3 hole-punch coordination: a joiner is asking the
       * tracker to nudge a registered host into firing UDP packets at
       * the joiner's reflexive address. Joiner's reflexive addr comes
       * from recvfrom; target is in bytes 8..13. */
      uint32_t targetIp;
      uint16_t targetPort;
      memcpy(&targetIp, buff + 8, 4);          /* already network order */
      targetPort = ((uint16_t)(uint8_t)buff[12] << 8) |
                   ((uint16_t)(uint8_t)buff[13]);
      fprintf(stderr, "[punch_req]\n");
      udpHandlePunchRequest(cg,
                            (uint32_t)last.sin_addr.s_addr,
                            ntohs(last.sin_port),
                            targetIp, targetPort);
    } else if (len == sizeof(INFO_PACKET)) {
      fprintf(stderr, "[ip,");
      if (strncmp(buff, BOLO_SIGNITURE, BOLO_SIGNITURE_SIZE) == 0 && buff[BOLOPACKET_PACKET_TYPEPOS] == BOLOPACKET_INFORESPONSE) {
	      INFO_PACKET *ipkt = (INFO_PACKET *)buff;

	      /* Process response */
	      fprintf(stderr, "assign,");

	      /* print a line in the infopacket log file */
	      {
		       char mapName[MAP_STR_SIZE]; /* Name of the map */
		       char timeStr[MAP_STR_SIZE]; /* time str */
		       BYTE ip[4];
		       time_t currTime;

		       memcpy(ip, & (last.sin_addr.s_addr), 4);
		       time(&currTime);
		       thread_safe_ctime(&currTime, timeStr, sizeof(timeStr));
		       timeStr[strlen(timeStr)-1] = 0;

		       utilPtoCString(ipkt->mapname, mapName);
		       fprintf(INFOPACKET_LOG, "[%s] %d.%d.%d.%d port=%d starttime=%ld map=%s\n", ctime(&currTime),
				       ip[0], ip[1], ip[2], ip[3],
				       ipkt->gameid.serverport,
				       ipkt->gameid.start_time,
				       mapName
			      );
		       fflush(INFOPACKET_LOG);
	      }

	      if (ipkt->gameid.serveraddress.s_addr == 0) {
		      /* Historical path: client did not advertise its own
		       * external address. Use the UDP source. */
		      udpAssignToThread(cg, ipkt, &last);
	      } else if (ipkt->h.versionMajor > 1 ||
			 (ipkt->h.versionMajor == 1 && ipkt->h.versionMinor > 1) ||
			 (ipkt->h.versionMajor == 1 && ipkt->h.versionMinor == 1 && ipkt->h.versionRevision > 7)) {
		      /* WinBolo Phase 2c (>1.1.7) populates serveraddress
		       * with its UPnP/NAT-PMP/PCP-mapped external IP. Trust it. */
		      udpAssignToThread(cg, ipkt, &last);
	      } else {
		      /* Pre-1.1.8 clients are not expected to set a non-zero
		       * serveraddress; silently drop these as malformed. */
		      fprintf(stderr, "drop-legacy-nonzero]\n");
	      }
      }
    } else {
     fprintf(stderr, "len %d wanted %d]\n", len, sizeof(INFO_PACKET));
     /* Debug: Dump raw packet bytes to understand the difference */
     fprintf(stderr, "UDP: Raw packet dump (first 80 bytes):\n");
     for (int i = 0; i < len && i < 80; i++) {
         fprintf(stderr, "%02X ", (unsigned char)buff[i]);
         if ((i + 1) % 16 == 0) fprintf(stderr, "\n");
     }
     fprintf(stderr, "\n");
     /* Also show the Bolo signature check */
     fprintf(stderr, "UDP: Signature check: buff[0-3]=%02X %02X %02X %02X (expected 'Bolo' = 42 6F 6C 6F)\n",
             (unsigned char)buff[0], (unsigned char)buff[1], (unsigned char)buff[2], (unsigned char)buff[3]);
     fprintf(stderr, "UDP: Packet type at pos 7: %02X (expected INFO_RESPONSE=0x0E=%02X)\n",
             (unsigned char)buff[BOLOPACKET_PACKET_TYPEPOS], BOLOPACKET_INFORESPONSE);
    }
    len = recvfrom(udpSocket, buff, sizeof(buff), 0, (struct sockaddr *) &last, (socklen_t *) &szlast);
  }

  fprintf(stderr, "UDP: done\n");
}

/*********************************************************
*NAME:          udpAssignToThread
*AUTHOR:        Andrew Roth
*CREATION DATE: 03/03/25 YMD
*LAST MODIFIED: 00/10/01
*PURPOSE:
*  Copies game info to a thread's info so it doesn't get
*  overwritten by another thread
*
*ARGUMENTS:
*  cg - a pointer to the currentgames structure
*  info - The info packet
*  pack - Address the packet came from
*  index - The thread index to copy the values to
*********************************************************/
void copyGameToThread(currentGames *cg, INFO_PACKET *info, struct sockaddr_in *pack, int index) {
	if (udpThreadInfo[index] != NULL) {
		fprintf(stderr, "copyinfo,");
		udpThreadInfo[index]->cg = cg;
		memcpy(&(udpThreadInfo[index]->info), info, sizeof(udpThreadInfo[index]->info));
		memcpy(&(udpThreadInfo[index]->pack), pack, sizeof(udpThreadInfo[index]->pack));
	} else {
		fprintf(stderr, "UDP WARNING: udpThreadInfo[%d] is null while copying to it!", index);
	}
}

/*********************************************************
*NAME:          udpAssignToThread
*AUTHOR:        Andrew Roth
*CREATION DATE: 03/03/10 YMD
*LAST MODIFIED: 00/10/01
*PURPOSE:
* Assigns a connection to a thread and signals the thread
*
*ARGUMENTS:
*  cg - a pointer to the currentgames structure
*  info - The info packet
*  pack - Address the packet came from
*********************************************************/
void udpAssignToThread(currentGames *cg, INFO_PACKET *info, struct sockaddr_in *pack) {
	int search, ret;
#ifndef _WIN32
	pthread_t threadId;      /* Thread ID -- ignored but required for pthread_create */
#else
	HANDLE threadHandle;
	DWORD threadId;
#endif
	  
	for (search = 0; search < udpNumThreads; search++) {
		if (!(udpThreadInfo[search]->busy)) {
			udpThreadInfo[search]->busy = TRUE;
			copyGameToThread(cg, info, pack, search);
#ifdef _WIN32
			WakeConditionVariable(&(udpThreadInfo[search]->signal));
			/* BUG FIX: Removed LeaveCriticalSection - main thread never entered this critical section! */
#else
			pthread_cond_signal(&(udpThreadInfo[search]->signal));
			/* BUG FIX: Removed pthread_mutex_unlock - main thread doesn't hold this mutex! */
#endif
			fprintf(stderr, "%d reuse]\n", search);
			return;
		} else
			fprintf(stderr, "%d-busy,", search);
	}
	
	if (udpNumThreads < UDP_MAX_THREADS) {
		/* start another thread... */
		udpThreadInfo[udpNumThreads] = (udpThreadInfoStruct *)malloc(sizeof(udpThreadInfoStruct));
		udpThreadInfo[udpNumThreads]->busy = TRUE;
		copyGameToThread(cg, info, pack, udpNumThreads);
		fprintf(stderr, "%d new]\n", udpNumThreads);
				
#ifdef _WIN32
		InitializeConditionVariable(&(udpThreadInfo[udpNumThreads]->signal));
		InitializeCriticalSection(&(udpThreadInfo[udpNumThreads]->mutex));
		threadHandle = CreateThread(NULL, 0, (LPTHREAD_START_ROUTINE)udpThreadWaitForWork, (void *)(intptr_t)udpNumThreads, 0, &threadId);
		if (threadHandle != NULL) {
			CloseHandle(threadHandle);  /* We don't need to keep the handle */
		}
#else
		pthread_cond_init(&(udpThreadInfo[udpNumThreads]->signal), NULL);
		pthread_mutex_init(&(udpThreadInfo[udpNumThreads]->mutex), NULL);
		ret = pthread_create(&threadId, NULL, udpThreadWaitForWork, (void *)(intptr_t)udpNumThreads);
#endif
		udpNumThreads++;
	} else {
		/* wait until a thead is done */
	}

	
}

/*********************************************************
*NAME:          udpThreadWaitForWork
*AUTHOR:        Andrew Roth
*CREATION DATE: 03/03/10 YMD
*LAST MODIFIED:
*PURPOSE:
* Thread waits for a connection to be assigned to it
*
*ARGUMENTS:
* udpThreadIndex - the thread index as integer, >= 0
*********************************************************/
#ifdef _WIN32
DWORD WINAPI udpThreadWaitForWork(LPVOID udpThreadIndex) {
	int myIndex = (int)(intptr_t)udpThreadIndex;
#else
void *udpThreadWaitForWork(void *udpThreadIndex) {
	int myIndex = (int)(intptr_t)udpThreadIndex;
#endif
	fprintf(stderr, "\tUDP: thread %d, ", myIndex);

	if (udpThreadInfo[myIndex]->busy) {
		fprintf(stderr, "startup\n");
		/* this must be initial startup, have work to do already! */
		udpProcessInfoPacket(udpThreadInfo[myIndex]->cg,
				&(udpThreadInfo[myIndex]->info),
				&(udpThreadInfo[myIndex]->pack));
	}

	while (TRUE) {
		/* wait for signal */
                fprintf(stderr, "\tUDP: thread %d sleep\n", myIndex);
		
#ifdef _WIN32
		EnterCriticalSection(&(udpThreadInfo[myIndex]->mutex));
		udpThreadInfo[myIndex]->busy = FALSE;
		SleepConditionVariableCS(&(udpThreadInfo[myIndex]->signal), &(udpThreadInfo[myIndex]->mutex), INFINITE);
		LeaveCriticalSection(&(udpThreadInfo[myIndex]->mutex));
#else
		pthread_mutex_lock(&(udpThreadInfo[myIndex]->mutex));
		udpThreadInfo[myIndex]->busy = FALSE;
		pthread_cond_wait(&(udpThreadInfo[myIndex]->signal), &(udpThreadInfo[myIndex]->mutex));
		pthread_mutex_unlock(&(udpThreadInfo[myIndex]->mutex));
#endif

		fprintf(stderr, "\tUDP: thread %d wakeup\n", myIndex);

		/* reaches here means there is a signal to process
		 * NOTE: udpAssignToThread will set busy to TRUE */
		udpProcessInfoPacket(udpThreadInfo[myIndex]->cg,
				&(udpThreadInfo[myIndex]->info),
				&(udpThreadInfo[myIndex]->pack));
	}
#ifdef _WIN32
	return 0;
#else
	return NULL;
#endif
}

/*********************************************************
*NAME:          udpProcessInfoPacket
*AUTHOR:        John Morrison and Andrew
*CREATION DATE: 00/10/01
*LAST MODIFIED: 01/03/21
*PURPOSE:
* Processes a game packet
*
*ARGUMENTS:
*  cg   - Pointer to current games structure
*  info - The info packet
*  pack - Address the packet came from
*********************************************************/
void udpProcessInfoPacket(currentGames *cg, INFO_PACKET *info, struct sockaddr_in *pack) {
  char mapName[MAP_STR_SIZE]; /* Name of the map */
  char version[MAP_STR_SIZE]; /* WinBolo Version */
  bool password;              /* Has Password */
  bool mines;                 /* Has mines */
  char address[FILENAME_MAX]; /* Address */
  struct in_addr registered;  /* address we register the game under */

  fprintf(stderr, "\tUDP: [processinfo,\n");

  /* If the client populated serveraddress (e.g. a UPnP-mapped external
   * IP), trust their advertised value; otherwise fall back to the UDP
   * source -- the historical behaviour for clients that sent zero. The
   * version gate that decides whether a non-zero value is acceptable
   * is enforced in udpRead before we get here. */
  if (info->gameid.serveraddress.s_addr != 0) {
    registered = info->gameid.serveraddress;
  } else {
    registered = pack->sin_addr;
  }

  utilReverseLookup(&registered, address);
  fprintf(stderr, "\t      ");
  if (bansExist(address) == TRUE) {
    fprintf(stderr, "banned]\n");
    return;
  }
  fprintf(stderr, "noban,");

  utilPtoCString(info->mapname, mapName);
  password = (info->has_password != 0);
  mines = (info->allow_mines == 0x80);
  sprintf(version, "%d.%d%d", info->h.versionMajor, info->h.versionMinor, info->h.versionRevision);

  /* Legacy v1.1.1-3 wire format byteswapped serverport; later versions
   * (incl. all current WinBolo + WinBoloDS) send raw. Preserve the
   * existing version-gated swap. */
  if (info->h.versionMajor == 1 && info->h.versionMinor == 1 &&
      (info->h.versionRevision == 1 || info->h.versionRevision == 2 ||
       info->h.versionRevision == 3)) {
    info->gameid.serverport = ntohs(info->gameid.serverport);
  }
  fprintf(stderr, "update,\n");
  currentGamesUpdate(cg, address, info->gameid.serverport, mapName, version,
                     (BYTE) info->num_players, (BYTE) info->free_bases,
                     (BYTE) info->free_pills, mines, info->gametype,
                     info->allow_AI, password,
                     htonl(info->gameid.start_time),
                     info->start_delay, info->time_limit,
                     (unsigned char *)&(registered.s_addr),
                     (unsigned char *)&(pack->sin_addr.s_addr),
                     ntohs(pack->sin_port));
  fprintf(stderr, "\n\t      ");
  fprintf(stderr, "done]\n");
}

