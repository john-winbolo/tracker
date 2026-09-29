/*********************************************************
*Name:          interesting
*Filename:      interesting.h
*Author:        John Morrison
*Creation Date: 00/10/01
*LAST MODIFIED: 00/10/01
*Purpose:
*  Responsable for handling of incoming interesting game list
*  connections
*********************************************************/

#ifndef _INTERESTING_H
#define _INTERESTING_H

#include "global.h"
#include "currentgames.h"
#include "stats.h"

/*********************************************************
*NAME:          interestingCreate
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: 00/10/01
*PURPOSE:
* Sets up the interesting module and returns the socket handle
*
*ARGUMENTS:
*  port - Port to bind to
*********************************************************/
SOCKET interestingCreate(unsigned short port);

/*********************************************************
*NAME:          interestingDestroy
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: 00/10/01
*PURPOSE:
* Cleansup after the interesting module
*
*ARGUMENTS:
*
*********************************************************/
void interestingDestroy();

/*********************************************************
*NAME:          interestingRead
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: 00/10/01
*PURPOSE:
* Checks the socket for new connections
*
*ARGUMENTS:
*  cg - Pointer to current games structure
*********************************************************/
void interestingRead(currentGames *cg);

/*********************************************************
*NAME:          interestingSendGameData
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: 00/10/01
*PURPOSE:
* Sends the game data
*
*ARGUMENTS:
*  cg   - Pointer to current games structure
*  sock - The socket to send to
*  pack - Address the packet came from
*********************************************************/
void interestingSendGameData(currentGames *cg, int sock, struct in_addr *pack);

#endif /* _interesting_H */

