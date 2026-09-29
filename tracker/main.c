/*
 * main.c - Modified to use epoll instead of select
 * This version uses epoll on Linux and WSAPoll on Windows
 */

#ifdef _WIN32
/* Windows winsock */
#include <winsock2.h>
#include <windows.h>
#include <memory.h>
#else
/* Linux headers */
#include <stdio.h>  
#include <stdlib.h>  
#include <netdb.h>  
#include <sys/types.h>  
#include <sys/time.h>  
#include <sys/socket.h>  
#include <time.h>  
#include <signal.h>  
#include <pthread.h>  
#include <string.h>  
#include <errno.h> 
#include <netinet/in.h>
#include <sys/epoll.h>
#include <unistd.h>

#define closesocket(X) close(X)
typedef struct hostent  HOSTENT;
#define SD_BOTH 2
#define INVALID_SOCKET -1
#define SOCKET_ERROR -1
#endif

#include "global.h"
#include "currentgames.h"
#include "udp.h"
#include "tcp.h"
#include "http.h"
#include "bans.h"
#include "main.h"
#include "irc.h"
#include "interesting.h"

#define MAX_EVENTS 64
#define PURGE_INTERVAL 30  /* Purge games every 30 seconds */

currentGames cg; /* Current Games */
bool mainQuit;   /* Should the program quit */
SOCKET mainUdp, mainTcp, mainHttp, mainDebug, mainIrc, mainInteresting;

long statsGames, statsTcp, statsUdp, statsHttp, statsInteresting;

#ifndef _WIN32
/* Linux epoll specific */
int epoll_fd = -1;

typedef struct socket_info {
    SOCKET sock;
    void (*handler)(currentGames*);  /* Handler function for this socket */
    const char* name;                 /* Socket name for debugging */
} socket_info;

/* Handler wrapper functions */
void handle_udp(currentGames* cg) { udpRead(cg); }
void handle_tcp(currentGames* cg) { tcpRead(cg); }
void handle_http(currentGames* cg) { httpRead(cg); }
void handle_interesting(currentGames* cg) { interestingRead(cg); }

int epoll_add_socket(SOCKET sock, void* user_data) {
    struct epoll_event ev;
    socket_info* info = (socket_info*)user_data;
    
    ev.events = EPOLLIN | EPOLLERR;
    ev.data.ptr = user_data;
    
    fprintf(stderr, "EPOLL: Adding socket %d (%s) to epoll fd %d\n", (int)sock, info ? info->name : "unknown", epoll_fd);
    
    if (epoll_ctl(epoll_fd, EPOLL_CTL_ADD, sock, &ev) == -1) {
        perror("epoll_ctl: add");
        return -1;
    }
    fprintf(stderr, "EPOLL: Successfully added socket %d\n", (int)sock);
    return 0;
}

#else
/* Windows WSAPoll specific */
#define MAX_SOCKETS 10
WSAPOLLFD poll_fds[MAX_SOCKETS];
int num_poll_fds = 0;

int poll_add_socket(SOCKET sock) {
    if (num_poll_fds >= MAX_SOCKETS) {
        fprintf(stderr, "Too many sockets for poll\n");
        return -1;
    }
    
    poll_fds[num_poll_fds].fd = sock;
    /* NOTE: POLLERR, POLLHUP, POLLNVAL are only valid in revents (output),
     * not in events (input). Setting them in events causes WSAPoll to
     * return WSAEINVAL (error 10022). They will be automatically reported
     * in revents when they occur. */
    poll_fds[num_poll_fds].events = POLLIN;
    poll_fds[num_poll_fds].revents = 0;
    num_poll_fds++;
    return 0;
}
#endif

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
void mainTellQuit() {
    mainQuit = TRUE;
}

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
void mainFlushGamesList() {
    currentGamesDestroy(&cg);
    cg = currentGamesCreate();
}

/*********************************************************
*NAME:          setup
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: Modified for epoll
*PURPOSE:
* Main program setup. Sets up each subsystem and returns
* overall success
*
*ARGUMENTS:
* 
*********************************************************/
bool setup() {
    bool returnValue = TRUE;

#ifdef _WIN32
    /* For winsock */
    WSADATA wsaData;
    if (WSAStartup(MAKEWORD(2,0), &wsaData) != 0) {
        fprintf(stderr, "Error Starting Winsock\n");
        returnValue = FALSE;
    }
#else
    /* Create epoll instance */
    epoll_fd = epoll_create1(EPOLL_CLOEXEC);
    if (epoll_fd == -1) {
        perror("epoll_create1");
        returnValue = FALSE;
    }
#endif

    statsGames = 0;
    statsTcp = 0;
    statsUdp = 0;
    statsHttp = 0;
    statsInteresting = 0;
    mainQuit = FALSE;
    
    cg = currentGamesCreate();
    bansCreate(NULL);

    /* Setup UDP socket */
    mainUdp = udpCreate(50000);
    if (mainUdp == INVALID_SOCKET) {
        fprintf(stderr, "MAIN: ERROR - Can't create udp socket\n");
        returnValue = FALSE;
    } else {
        fprintf(stderr, "MAIN: UDP socket created, fd=%d\n", (int)mainUdp);
#ifndef _WIN32
        socket_info* info = malloc(sizeof(socket_info));
        info->sock = mainUdp;
        info->handler = handle_udp;
        info->name = "UDP";
        if (epoll_add_socket(mainUdp, info) == 0) {
            fprintf(stderr, "MAIN: UDP socket added to epoll successfully\n");
        } else {
            fprintf(stderr, "MAIN: ERROR - Failed to add UDP socket to epoll\n");
        }
#else
        if (poll_add_socket(mainUdp) == 0) {
            fprintf(stderr, "MAIN: UDP socket added to poll successfully\n");
        } else {
            fprintf(stderr, "MAIN: ERROR - Failed to add UDP socket to poll\n");
        }
#endif
    }

    /* Setup TCP socket */
    mainTcp = tcpCreate(50000);
    if (mainTcp == INVALID_SOCKET) {
        printf("Can't create tcp socket\n");
        returnValue = FALSE;
    } else {
#ifndef _WIN32
        socket_info* info = malloc(sizeof(socket_info));
        info->sock = mainTcp;
        info->handler = handle_tcp;
        info->name = "TCP";
        epoll_add_socket(mainTcp, info);
#else
        poll_add_socket(mainTcp);
#endif
    }

    /* Setup HTTP socket */
    mainHttp = httpCreate(50005, NULL);
    if (mainHttp == INVALID_SOCKET) {
        printf("Can't create http socket\n");
        returnValue = FALSE;
    } else {
#ifndef _WIN32
        socket_info* info = malloc(sizeof(socket_info));
        info->sock = mainHttp;
        info->handler = handle_http;
        info->name = "HTTP";
        epoll_add_socket(mainHttp, info);
#else
        poll_add_socket(mainHttp);
#endif
    }

    /* Setup interesting games socket */
    mainInteresting = interestingCreate(50001);
    if (mainInteresting == INVALID_SOCKET) {
        printf("Can't create interesting socket\n");
        returnValue = FALSE;
    } else {
#ifndef _WIN32
        socket_info* info = malloc(sizeof(socket_info));
        info->sock = mainInteresting;
        info->handler = handle_interesting;
        info->name = "Interesting";
        epoll_add_socket(mainInteresting, info);
#else
        poll_add_socket(mainInteresting);
#endif
    }
    
    return returnValue;
}

/*********************************************************
*NAME:          destroy
*AUTHOR:        John Morrison
*CREATION DATE: 00/10/01
*LAST MODIFIED: Modified for epoll
*PURPOSE:
* Main program destroy. Cleans up each subsystem 
*
*ARGUMENTS:
* 
*********************************************************/
void destroy() {
    udpDestroy();
    tcpDestroy();
    httpDestroy();
    interestingDestroy();

    currentGamesDestroy(&cg);
    bansDestroy(NULL);
    
#ifndef _WIN32
    if (epoll_fd != -1) {
        close(epoll_fd);
    }
#endif

#ifdef _WIN32
    WSACleanup();
#endif
}

/*********************************************************
*NAME:          main
*AUTHOR:        John Morrison and Andrew Roth
*CREATION DATE: 00/10/01
*LAST MODIFIED: Modified to use epoll/WSAPoll
*PURPOSE:
* Main program loop
*
*ARGUMENTS:
* 
*********************************************************/
int main(int argc, char **argv) {
    time_t last_purge = time(NULL);
    
    /* Set stderr to unbuffered for immediate log output */
    setvbuf(stderr, NULL, _IONBF, 0);
    
    if (setup() == FALSE) {
        fprintf(stderr, "Error starting up\n");
        exit(1);
    }

#ifndef _WIN32
    /* Linux epoll main loop */
    struct epoll_event events[MAX_EVENTS];
    
    while (mainQuit == FALSE) {
        /* Calculate timeout for purging (in milliseconds) */
        time_t now = time(NULL);
        int timeout_ms = (PURGE_INTERVAL - (now - last_purge)) * 1000;
        if (timeout_ms < 0) timeout_ms = 0;
        if (timeout_ms > PURGE_INTERVAL * 1000) timeout_ms = PURGE_INTERVAL * 1000;
        
        int nfds = epoll_wait(epoll_fd, events, MAX_EVENTS, timeout_ms);
        
        fprintf(stderr, "EPOLL: epoll_wait returned %d (timeout=%dms)\n", nfds, timeout_ms);
        
        if (nfds == -1) {
            if (errno == EINTR) {
                fprintf(stderr, "EPOLL: Interrupted by signal, retrying\n");
                continue;  /* Interrupted by signal, retry */
            }
            perror("epoll_wait");
            break;
        }
        
        /* Handle events */
        for (int i = 0; i < nfds; i++) {
            socket_info* info = (socket_info*)events[i].data.ptr;
            
            fprintf(stderr, "EPOLL: Event %d - socket=%d, name=%s, events=0x%x\n",
                    i, info ? (int)info->sock : -1, info ? info->name : "null", events[i].events);
            
            if (events[i].events & EPOLLERR) {
                fprintf(stderr, "EPOLL: Error on socket %s\n", info->name);
                continue;
            }
            
            if (events[i].events & EPOLLIN) {
                fprintf(stderr, "EPOLL: EPOLLIN on %s socket, calling handler\n", info->name);
                info->handler(&cg);
            }
        }
        
        /* Periodic game purging */
        now = time(NULL);
        if (now - last_purge >= PURGE_INTERVAL) {
            fprintf(stderr, "MAIN: Purging old games\n");
            currentGamesPurge(&cg);
            last_purge = now;
        }
    }
#else
    /* Windows WSAPoll main loop */
    while (mainQuit == FALSE) {
        time_t now = time(NULL);
        int timeout_ms = (PURGE_INTERVAL - (now - last_purge)) * 1000;
        if (timeout_ms < 0) timeout_ms = 0;
        if (timeout_ms > PURGE_INTERVAL * 1000) timeout_ms = PURGE_INTERVAL * 1000;
        
        fprintf(stderr, "WSAPoll: calling with %d sockets, timeout=%dms\n", num_poll_fds, timeout_ms);
        fflush(stderr);
        for (int i = 0; i < num_poll_fds; i++) {
            fprintf(stderr, "  [%d] fd=%d, events=0x%x, revents=0x%x\n",
                    i, (int)poll_fds[i].fd, poll_fds[i].events, poll_fds[i].revents);
        }
        fflush(stderr);
        
        int ret = WSAPoll(poll_fds, num_poll_fds, timeout_ms);
        
        fprintf(stderr, "WSAPoll: returned %d\n", ret);
        fflush(stderr);
        
        if (ret == SOCKET_ERROR) {
            fprintf(stderr, "WSAPoll error: %d\n", WSAGetLastError());
            break;
        }
        
        if (ret > 0) {
            /* Check each socket for activity */
            for (int i = 0; i < num_poll_fds; i++) {
                fprintf(stderr, "  [%d] fd=%d, revents=0x%x (POLLIN=0x%x)\n",
                        i, (int)poll_fds[i].fd, poll_fds[i].revents, POLLIN);
                if (poll_fds[i].revents & POLLERR) {
                    fprintf(stderr, "Error on socket\n");
                    continue;
                }
                
                if (poll_fds[i].revents & POLLIN) {
                    /* Determine which handler to call based on socket */
                    if (poll_fds[i].fd == mainUdp) {
                        fprintf(stderr, "Activity on UDP socket\n");
                        udpRead(&cg);
                    } else if (poll_fds[i].fd == mainTcp) {
                        fprintf(stderr, "Activity on TCP socket\n");
                        tcpRead(&cg);
                    } else if (poll_fds[i].fd == mainHttp) {
                        fprintf(stderr, "Activity on HTTP socket\n");
                        httpRead(&cg);
                    } else if (poll_fds[i].fd == mainInteresting) {
                        fprintf(stderr, "Activity on Interesting socket\n");
                        interestingRead(&cg);
                    }
                }
            }
        }
        
        /* Periodic game purging */
        now = time(NULL);
        if (now - last_purge >= PURGE_INTERVAL) {
            fprintf(stderr, "MAIN: Purging old games\n");
            currentGamesPurge(&cg);
            last_purge = now;
        }
    }
#endif

    destroy();
    return 0;
}

