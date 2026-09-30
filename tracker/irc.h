/* SPDX-License-Identifier: GPL-3.0-or-later */
/*********************************************************
*Name:          debug
*Filename:      debug.h
*Author:        John Morrison
*Creation Date: 00/10/01
*LAST MODIFIED: 00/10/02
*Purpose:
*  Responsable for handling of debug telnet connections
*********************************************************/

#ifndef _IRC_H
#define _IRC_H

#include "global.h"

#define IsEmpty(list) ((list) ==NULL)
#define NonEmpty(list) (!IsEmpty(list))
#define IrcTail(list) ((list)->next)

#define	IRC_READ_LEN	512

#define	IRC_SEND_TXT	"/nick clkdsl\n/join #testing\n/say hi"
#define	IRC_MAX_GAME_LEN	512

/* Each debug client consists of a socket identifier
 * and a text buffer for inputing */
typedef struct ircClientsObj *ircClients;
struct ircClientsObj {
  ircClients next;          /* Next item */
  ircClients prev;          /* Prev item */
  SOCKET sock;                /* socket    */
};

/*********************************************************
*NAME:          ircCreate
*AUTHOR:        Andrew Roth
*CREATION DATE: 03/03/29 YMD
*LAST MODIFIED: 03/03/29
*PURPOSE:
* Sets up the irc listener port module and returns the
* socket handle
*
*ARGUMENTS:
*  port     - Port to bind to
*********************************************************/
SOCKET ircCreate(unsigned short port, char *fileName);

/*********************************************************
*NAME:          ircDestroy
*AUTHOR:        Andrew Roth
*CREATION DATE: 03/03/29 YMD
*LAST MODIFIED: 03/03/29
*PURPOSE:
* Cleansup after the irc module
*
*ARGUMENTS:
*
*********************************************************/
void ircDestroy();

/*********************************************************
*NAME:          ircRead
*AUTHOR:        Andrew Roth
*CREATION DATE: 03/03/29 YMD
*LAST MODIFIED: 03/03/29
*PURPOSE:
* Checks the socket for new connections and all its
* subclients
*
*ARGUMENTS:
*
*********************************************************/
void ircRead();

/*********************************************************
*NAME:          ircGetHighestSocket
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/02
*LAST MODIFIED: 00/10/02
*PURPOSE:
* Returns the highest socket in se by the debug subsystem
*
*ARGUMENTS:
*
*********************************************************/
int ircGetHighestSocket();

/*********************************************************
*NAME:          ircAddNewClient
*AUTHOR:        Andrew Roth
*CREATION DATE: 03/03/29
*LAST MODIFIED: 03/03/29
*PURPOSE:
* Adds a new client to the list of active irc clients
*
*ARGUMENTS:
*  sock - New socket to add
*  addr - Address of the client
*********************************************************/
void ircAddNewClient(SOCKET sock, struct in_addr *addr);

/*********************************************************
*NAME:          ircCheckClients
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: 00/10/02
*PURPOSE:
* Checks each of the connected irc clients for new data
*
*ARGUMENTS:
*
*********************************************************/
void ircCheckClients();

/*********************************************************
*NAME:          ircSendClientsMessage
*AUTHOR:        Andrew Roth
*CREATION DATE: 00/10/02
*LAST MODIFIED: 00/10/02
*PURPOSE:
* Sends a message to all clients
*
*ARGUMENTS:
*  text   - Text to send
*********************************************************/
void ircSendClientsMessage(char *text);

/*********************************************************
 *NAME:          ircRemoveClient
 *AUTHOR:        Andrew Roth
 *CREATION DATE: 03/03/29 YMD
 *LAST MODIFIED: 03/03/29
 *PURPOSE:
 * Removes a client from client linked list
 *
 *ARGUMENTS:
 *  q2 yeah, something should go here
 *********************************************************/
void ircRemoveClient(ircClients *q2);

#endif  /* _IRC_H */
