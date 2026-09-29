/*********************************************************
*Name:          currentGames
*Filename:      currentGames.h
*Author:        John Morrison
*Creation Date: 00/01/18
*LAST MODIFIED: 00/10/01
*Purpose:
*  Responsable for holding the list of current games
*********************************************************/


#ifndef _CURRENT_GAMES_H
#define _CURRENT_GAMES_H


#include <time.h>
#ifdef _WIN32
#include <windows.h>
#else
#include <pthread.h>
#endif
#include "global.h"
#include "irc.h"


/* Games expire after 4 minutes, 10 seconds of no updates 
   As far as I can see servers send info every 4 minutes
   The new timeout is 2 mins, 10 secs for >= 1.09  -Andrew
   Now put back to 4, in effort to fix games dropping
   off the tracker -Andrew */
#define CURRENT_GAME_TRACKER_EXPIRE_OLD (4 * 60) + 10
#define CURRENT_GAME_TRACKER_EXPIRE_NEW (4 * 60) + 10

#define IsEmpty(list) ((list) ==NULL)
#define NonEmpty(list) (!IsEmpty(list))
#define CurrentTail(list) ((list)->next);

/* Type structure */

typedef struct currentGamesObj *currentGames;
struct currentGamesObj {
  currentGames next; /* Next item */
  char address[FILENAME_MAX];
  char mapName[MAP_STR_SIZE];
  char version[FILENAME_MAX];
  unsigned short port;
  BYTE numPlayers;
  BYTE numBases;
  BYTE numPills;
  bool mines;
  gameType game;
  aiType ai;
  bool password;
  unsigned long starttime;
  long startdelay;
  long timelimit;
  time_t lastpacket;
  unsigned char ip[IP_SIZE];           /* registered/advertised IP */
  unsigned long gameLogOffset;
  /* Source address of the most recent UDP packet from this game.
   * Updated on INFO_PACKET registration AND on 'WBKA' keepalive.
   * Used to push PACKET_PUNCH_NOTIFY through the host's live NAT
   * mapping when a joiner requests a hole-punch. */
  unsigned char sourceIp[IP_SIZE];
  unsigned short sourcePort;
};

/* Prototypes */

/*********************************************************
 *NAME:          currentGamesCreateIrcString
 *AUTHOR:        Andrew Roth
 *CREATION DATE: 03/03/29 YMD
 *LAST MODIFIED: 03/03/29
 *PURPOSE:
 *  Creates the irc string into str
 *
 *  Note the currentGames structure is not locked in this
 *  method but it should be locked by a calling method
 *
 *ARGUMENTS:
 *  str - a pointer to the irc string
 *  cg - a pointer to the currentGames structure
 *********************************************************/
void currentGamesCreateIrcString(char str[IRC_MAX_GAME_LEN], currentGames cg);

/*********************************************************
*NAME:          currentGamesCreate
*AUTHOR:        John Morrison
*CREATION DATE: 00/01/18
*LAST MODIFIED: 00/01/18
*PURPOSE:
*  Sets up the currentGames data structure
*
*ARGUMENTS:
*
*********************************************************/
currentGames currentGamesCreate(void);

/*********************************************************
 *NAME:          currentGamesLock
 *AUTHOR:        Andrew Roth
 *CREATION DATE: 03/01/22
 *LAST MODIFIED: 03/01/22
 *PURPOSE:
 *  Waits until a lock is obtained on the currentGames
 *  structure.  When it is finished with the currentGames
 *  structure, it should call currentGamesUnlock
 *
 *ARGUMENTS:
 **********************************************************/
void currentGamesLock(void);

/*********************************************************
 *NAME:          currentGamesUnlock
 *AUTHOR:        Andrew Roth
 *CREATION DATE: 03/01/22
 *LAST MODIFIED: 03/01/22
 *PURPOSE:
 *  unlocks the currentgames structure.  only a thread
 *  which has a lock should do this
 *
 *ARGUMENTS:
 **********************************************************/
void currentGamesUnlock(void);

/*********************************************************
*NAME:          currentGamesDestroy
*AUTHOR:        John Morrison
*CREATION DATE: 00/01/18
*LAST MODIFIED: 00/01/18
*PURPOSE:
*  Destroys the currentGames data structure
*
*ARGUMENTS:
*  value - The structure to destroy
*********************************************************/
void currentGamesDestroy(currentGames *value);

/*********************************************************
*NAME:          currentGamesUpdate
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: 00/10/01
*PURPOSE:
* This function adds a game to the game list. (or updates
* if it exists) As it goes through the list it deletes
* expired games.
*
*ARGUMENTS:
*  value      - The currentGames structure
*  address    - Server address
*  port       - Server port
*  mapName    - Name of the map
*  version    - Game version
*  numPlayers - Number of players
*  numBases   - Number of bases
*  numPills   - Number of pills
*  mines      - Allow hidden mines
*  game       - Game type
*  ai         - Ai type
*  password   - Has password
*  starttime  - Time the game started
*  startdelay - Start delay time
*  timelimit  - Game time limit
*  ip         - 4 byte registered/advertised IP for stats purposes
*  sourceIp   - 4 byte source IP of the UDP packet (may differ from ip
*               when host advertises a UPnP-mapped external address)
*  sourcePort - source port of the UDP packet (host byte order)
*********************************************************/
void currentGamesUpdate(currentGames *value, char *address, unsigned short port, char *mapName, char *version, BYTE numPlayers, BYTE numBases, BYTE numPills, bool mines, gameType game, aiType ai, bool password, unsigned long starttime, long startdelay, long timelimit, unsigned char *ip, unsigned char *sourceIp, unsigned short sourcePort);

/*********************************************************
*NAME:          currentGamesAddItem
*AUTHOR:        John Morrison
*CREATION DATE: 00/01/18
*LAST MODIFIED: 00/10/01
*PURPOSE:
*  Adds an item to the currentGames data structure.
*  Updates if it exists
*
*ARGUMENTS:
*  value      - The currentGames structure
*  address    - Server address
*  port       - Server port
*  mapName    - Name of the map
*  version    - Game version
*  numPlayers - Number of players
*  numBases   - Number of bases
*  numPills   - Number of pills
*  mines      - Allow hidden mines
*  game       - Game type
*  ai         - Ai type
*  password   - Has password
*  starttime  - Time the game started
*  startdelay - Start delay time
*  timelimit  - Game time limit
*  ip         - 4 byte registered/advertised IP for stats purposes
*  sourceIp   - 4 byte source IP of the UDP packet
*  sourcePort - source port of the UDP packet (host byte order)
*********************************************************/
void currentGamesAddItem(currentGames *value, char *address, unsigned short port, char *mapName, char *version, BYTE numPlayers, BYTE numBases, BYTE numPills, bool mines, gameType game, aiType ai, bool password, unsigned long starttime, long startdelay, long timelimit, unsigned char *ip, unsigned char *sourceIp, unsigned short sourcePort);

/*********************************************************
*NAME:          currentGamesRefreshSource
*PURPOSE:
*  Refreshes the live NAT source mapping for any games
*  whose stored sourceIp matches sourceAddr. Called when
*  a 'WBKA' keepalive arrives. Updates sourcePort and
*  lastpacket; sourceIp is left as-is (matched on).
*
*ARGUMENTS:
*  value      - The currentGames structure
*  sourceAddr - source address (network byte order, raw s_addr)
*  sourcePort - source port (host byte order)
*********************************************************/
void currentGamesRefreshSource(currentGames *value, unsigned long sourceAddr, unsigned short sourcePort);

/*********************************************************
*NAME:          currentGamesRefreshSourceExact
*PURPOSE:
*  Refreshes the live NAT source mapping for the single
*  game whose stored sourceIp matches sourceAddr AND whose
*  starttime matches startTime. Called when an 8-byte WBKA
*  keepalive carrying a game token arrives. Disambiguates
*  the case where one NAT IP hosts more than one WinBolo
*  game; the 4-byte fallback (currentGamesRefreshSource)
*  remains for clients that haven't been updated yet.
*  Updates sourcePort and lastpacket; sourceIp is left as
*  is. Drops silently if no entry matches (e.g. stale
*  token from a re-registered host).
*
*ARGUMENTS:
*  value      - The currentGames structure
*  sourceAddr - source address (network byte order, raw s_addr)
*  sourcePort - source port (host byte order)
*  startTime  - game token == INFO_PACKET.gameid.start_time
*               as stored in q->starttime
*********************************************************/
void currentGamesRefreshSourceExact(currentGames *value, unsigned long sourceAddr, unsigned short sourcePort, unsigned long startTime);

/*********************************************************
*NAME:          currentGamesItemCount
*AUTHOR:        John Morrison
*CREATION DATE: 00/01/18
*LAST MODIFIED: 00/10/01
*PURPOSE:
*  Returns the number of elements in the structure. Also
*  purges empty games
*
*ARGUMENTS:
*  value - The currentGames structure
*********************************************************/
int currentGamesItemCount(currentGames *value);

/*********************************************************
*NAME:          currentGamesGetItem
*AUTHOR:        John Morrison
*CREATION DATE: 00/01/18
*LAST MODIFIED: 00/10/01
*PURPOSE:
*  Gets the item for an itemNum
*
*ARGUMENTS:
*  value      - The currentGames structure
*  itemNum    - Item number to get record for
*  address    - Server address
*  port       - Server port
*  mapName    - Name of the map
*  version    - Game version
*  numPlayers - Number of players
*  numBases   - Number of bases
*  numPills   - Number of pills
*  mines      - Allow hidden mines
*  game       - Game type
*  ai         - Ai type
*  password   - Has password
*  starttime  - Time the game started
*  startdelay - Start delay time
*  timelimit  - Game time limit
*********************************************************/
void currentGamesGetItem(currentGames *value, int itemNum, char *address, unsigned short *port, char *mapName, char *version, BYTE *numPlayers, BYTE *numBases, BYTE *numPills, bool *mines, gameType *game, aiType *ai, bool *password, unsigned long *starttime, long *startdelay, long *timelimit);

/*********************************************************
*NAME:          currentGamesGetServerName
*AUTHOR:        John Morrison
*CREATION DATE: 00/01/18
*LAST MODIFIED: 00/01/18
*PURPOSE:
*  Gets the server name for an itemNum
*
*ARGUMENTS:
*  value   - The currentGames structure
*  itemNum - Item number to get record for
*  address - Server address
*********************************************************/
void currentGamesGetServerName(currentGames *value, int itemNum, char *address);

/*********************************************************
*NAME:          currentGamesItemCount
*AUTHOR:        Andrew Roth
*CREATION DATE: 01/05/01
*LAST MODIFIED: 01/05/01
*PURPOSE:
*  purges timed out games
*
*ARGUMENTS:
*  value - The currentGames structure
*********************************************************/
void currentGamesPurge(currentGames *value);

#endif /* _CURRENTGAMES_H */

