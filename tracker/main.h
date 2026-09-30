/* SPDX-License-Identifier: GPL-3.0-or-later */
/*********************************************************
*Name:          main
*Filename:      main.h
*Author:        John Morrison
*Creation Date: 00/10/01
*LAST MODIFIED: 00/10/02
*Purpose:
*  Provides main glue
*********************************************************/

#ifndef _MAIN_H
#define _MAIN_H

extern long statsGames, statsTcp, statsUdp, statsHttp, statsInteresting;

#include "global.h"

/*********************************************************
*NAME:          mainRemoveSocket
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/02
*LAST MODIFIED: 00/10/02
*PURPOSE:
* Removes the socket from the read set
*
*ARGUMENTS:
*  sock - Socket to add
*********************************************************/
void mainRemoveSocket(SOCKET sock);

/*********************************************************
*NAME:          mainAddSocket
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/02
*LAST MODIFIED: 00/10/02
*PURPOSE:
* Adds the socket from the read set
*
*ARGUMENTS:
*  sock - Socket to add
*********************************************************/
void mainAddSocket(SOCKET sock);

/*********************************************************
*NAME:          mainFlushGamesList
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/02
*LAST MODIFIED: 00/10/02
*PURPOSE:
* Flushes the current game list
*
*ARGUMENTS:
* 
*********************************************************/
void mainFlushGamesList();

/*********************************************************
*NAME:          setup
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: 00/10/02
*PURPOSE:
* Main program setup. Sets up each subsystem and returns
* overall success
*
*ARGUMENTS:
* 
*********************************************************/
bool setup();

/*********************************************************
*NAME:          destroy
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: 00/10/02
*PURPOSE:
* Main program destroy. Cleans up each subsystem 
*
*ARGUMENTS:
* 
*********************************************************/
void destroy();

/*********************************************************
*NAME:          mainTellQuit
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/02
*LAST MODIFIED: 00/10/02
*PURPOSE:
* Tells the main program to quit
*
*ARGUMENTS:
* 
*********************************************************/
void mainTellQuit();

/*********************************************************
*NAME:          mainGetHighestSocket
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/02
*LAST MODIFIED: 00/10/02
*PURPOSE:
* Returns highest socket identifier
*
*ARGUMENTS:
* 
*********************************************************/
int mainGetHighestSocket();

#endif /* _MAIN_H */

