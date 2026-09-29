/*********************************************************
*Name:          udp
*Filename:      udp.h
*Author:        John Morrison
*Creation Date: 00/10/01
*LAST MODIFIED: 00/10/01
*Purpose:
*  Responsable for handling incoming UDP game update 
*  packets
*********************************************************/


#ifndef _UDP_H
#define _UDP_H

#ifdef _WIN32
/* Windows winsock */
#include <winsock2.h>
#include <windows.h>
#include <memory.h>

#else
/* Liunux Winsock plus some useful macros */
#include <sys/socket.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#define INVALID_SOCKET -1
#define SOCKET_ERROR -1
#include <netdb.h>
// #include <unistd.h>
#include <fcntl.h>
#include <errno.h>

#define closesocket(X) close(X)
/* typedef struct hostent  HOSTENT; */
#define SD_BOTH 2

#endif


extern int udpMaxThreads;

/* Packet description */
#define BOLOPACKET_INFORESPONSE  14
/* "Bolo" string and size of */
#define BOLO_SIGNITURE "Bolo"
#define BOLO_SIGNITURE_SIZE 4
#define BOLOPACKET_PACKET_TYPEPOS 7

/* Phase 3 -- UDP hole-punching coordination. New-protocol
 * 'WB' header convention (PACKET_HEADER_SIZE = 8); all
 * multi-byte fields big-endian. */
#define PACKET_PUNCH_REQUEST        153   /* joiner   -> tracker */
#define PACKET_PUNCH_NOTIFY         154   /* tracker  -> host    */
#define PACKET_PUNCH_REQUEST_ACK    155   /* tracker  -> joiner, status byte */
#define PACKET_PUNCH_PROBE_REQUEST  156   /* host     -> tracker, no body    */
#define PACKET_PUNCH_PROBE_REPLY    157   /* tracker  -> host, reflexive ip:port */

/* Status codes carried by PACKET_PUNCH_REQUEST_ACK. */
#define PUNCH_ACK_OK                0   /* host found, PUNCH_NOTIFY sent */
#define PUNCH_ACK_HOST_NOT_FOUND    1   /* no matching (ip, port) registered */
#define PUNCH_ACK_HOST_UNREACHABLE  2   /* registered but no live NAT mapping known */

/* Bolo header packets - packed for network protocol compatibility */
#pragma pack(push, 1)
typedef struct {
  BYTE signature[4];    /* 'Bolo'                */
  BYTE versionMajor;    /* BOLO_VERSION_MAJOR    */
  BYTE versionMinor;    /* BOLO_VERSION_MINOR    */
  BYTE versionRevision; /* BOLO_VERSION_REVISION */
  BYTE type;            /* Packet Type           */
} BOLOHEADER;

typedef struct {
	struct in_addr serveraddress;
	uint16_t serverport;      /* Fixed 2 bytes */
	uint16_t _padding;        /* explicit padding - matches original 76-byte wire format */
	uint32_t start_time;      /* Fixed 4 bytes */
} GAMEID;

typedef struct {
  BOLOHEADER h;

  char mapname[MAP_STR_SIZE]; /* Pascal string (first byte is length)         */
  GAMEID gameid;       /* 10 byte unique ID for game (combination      */
                       /* of starting machine address & timestamp)    */
  BYTE gametype;       /* Game type (1, 2 or 3: open, tourn. & strict) */
  BYTE allow_mines;    /* 0x80 for normal hidden mines                 */
                       /* 0xC0 for all mines visible                   */
  BYTE allow_AI;       /* 0 for no AI tanks, 1 for AI tanks allowed    */
  BYTE spare1;         /* 0                                            */
  int32_t start_delay; /* if non zero, time until game starts, (50ths) - Fixed 4 bytes */
  int32_t time_limit;  /* if non zero, time until game ends, (50ths) - Fixed 4 bytes */

  WORD num_players;    /* number of players                            */
  WORD free_pills;     /* number of free (neutral) pillboxes           */
  WORD free_bases;     /* number of free (neutral) refuelling bases    */
  BYTE has_password;   /* non-zero if game has password set            */
  BYTE spare2;         /* 0                                            */
} INFO_PACKET;
#pragma pack(pop)

/* Compile-time verification of structure sizes for network protocol compatibility */
#include <assert.h>
static_assert(sizeof(GAMEID) == 12, "GAMEID must be 12 bytes for network protocol compatibility");
static_assert(sizeof(INFO_PACKET) == 76, "INFO_PACKET must be 76 bytes for network protocol compatibility");


/* this structure stores mutex/signal and connection info for one thread */
#ifdef _WIN32
/* Windows uses CONDITION_VARIABLE and CRITICAL_SECTION */
typedef struct {
	int busy;
	currentGames *cg;
	INFO_PACKET info;
	struct sockaddr_in pack;   /* source addr+port of the UDP packet */
	CONDITION_VARIABLE signal;
	CRITICAL_SECTION mutex;
} udpThreadInfoStruct;
#else
/* Linux uses pthread */
typedef struct {
	int busy;
	currentGames *cg;
	INFO_PACKET info;
	struct sockaddr_in pack;   /* source addr+port of the UDP packet */
	pthread_cond_t signal;
	pthread_mutex_t mutex;
} udpThreadInfoStruct;
#endif
extern int udpNumThreads;

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
SOCKET udpCreate(unsigned short port);

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
void udpDestroy();


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
void udpRead(currentGames *cg);

/*********************************************************
*NAME:          udpProcessInfoPacket
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: 00/10/01
*PURPOSE:
* Processes a game packet
*
*ARGUMENTS:
*  cg   - Pointer to current games structure
*  info - The info packet
*  pack - Source address+port the packet came from
*********************************************************/
void udpProcessInfoPacket(currentGames *cg, INFO_PACKET *info, struct sockaddr_in *pack);

/*********************************************************
*NAME:          udpAssignToThread
*AUTHOR:        Andrew Roth
*CREATION DATE: 03/03/10 YMD
*LAST MODIFIED: 00/10/01
*PURPOSE:
* Assigns a connection to a thread and signals the thread
*
*ARGUMENTS:
*  info - The info packet
*  pack - Source address+port the packet came from
*********************************************************/
void udpAssignToThread(currentGames *cg, INFO_PACKET *info, struct sockaddr_in *pack);

/*********************************************************
*NAME:          udpThreadWaitForWork
*AUTHOR:        Andrew Roth
*CREATION DATE: 03/03/10 YMD
*LAST MODIFIED:
*PURPOSE:
* Processes a game packet
*
*ARGUMENTS:
* connectionInfoVoid - pointer to connInfo structure which
*  holds everything necessary to call udpProcessInfoPacket
*********************************************************/
#ifdef _WIN32
DWORD WINAPI udpThreadWaitForWork(LPVOID connectionInfo);
#else
void *udpThreadWaitForWork(void *connectionInfo);
#endif

/*********************************************************
*NAME:          udpThreadWaitForWork
*AUTHOR:        Andrew Roth
*CREATION DATE: 03/03/10 YMD
*LAST MODIFIED:
*PURPOSE:
* Processes a game packet
*
*ARGUMENTS:
* connectionInfoVoid - pointer to connInfo structure which
*  holds everything necessary to call udpProcessInfoPacket
**********************************************************/
void *udpThreadSend(void *connectionInfo);

#endif /* _UDP_H */

