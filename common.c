/*
 ****************************************************************************
 * common.c
 *      widely used common routines.
 * 
 * (C) 2016 by Alexey V. Lesovsky (lesovsky <at> gmail.com)
 * 
 ****************************************************************************
 */
#include "include/common.h"

/*
 ****************************************************************************
 * If something goes wrong, print diagnostic message and exit from program 
 * if needed. Don't use that in the ncurses mode.
 ****************************************************************************
 */
void mreport(bool do_exit, enum mtype mtype, const char * msg, ...)
{
    FILE * out = stdout;
    int status = EXIT_SUCCESS;

    if (mtype == msg_fatal || mtype == msg_error) {
        out = stderr;
        status = EXIT_FAILURE;
    }

    /* detach stdout/stderr file descriptors from ncurses mode */
    endwin();

    va_list args;
    va_start(args, msg);
    vfprintf(out, msg, args);
    va_end(args);        
 
    if (do_exit) {
        exit(status);
    }
}

/*
 ****************************************************************************
 * Signal handler
 ****************************************************************************
 */
void sig_handler(int signo)
{
    switch (signo) {
        default: case SIGINT:
            endwin();
            exit(EXIT_SUCCESS);
            break;
    }
}

/*
 ****************************************************************************
 * Assign signal handlers to signals.
 ****************************************************************************
 */
void init_signal_handlers(void)
{
    if (signal(SIGINT, sig_handler) == SIG_ERR) {
        mreport(true, msg_fatal, "FATAL: failed to establish SIGINT handler.\n");
    }
}

/*
 ****************************************************************************
 * Replace substring in string.
 * Looking for the 's_string' inside the 'o_string' and replace it with the 
 * 'r_string' when it's found. Return modified the 'o_string'.
 ****************************************************************************
 */
void strrpl(char * o_string, const char * s_string, const char * r_string, unsigned int buf_size)
{
    char buffer[buf_size];
    char * ch;
             
    if(!(ch = strstr(o_string, s_string)))
        return;
    
    strncat(buffer, o_string, ch - o_string);
    sprintf(buffer + (ch - o_string), "%s%s", r_string, ch + strlen(s_string));
    o_string[0] = 0;
    snprintf(o_string, buf_size, "%s", buffer);
    strrpl(o_string, s_string, r_string, buf_size);

    return;
}

/*
 ****************************************************************************
 * Check that the string is satisfied to given type (number,float,string).
 * Return 0 if string is valid, -1 otherwise.
 ****************************************************************************
 */
int check_string(const char * string, enum chk_type ctype)
{
    unsigned int i;
    switch (ctype) {
        case is_alfanum:
            for (i = 0; string[i] != '\0'; i++) {
                if (!isalnum(string[i]))    /* non-alfanumeric char found */
                    return -1;
            }
        break;
        case is_number:
            for (i = 0; string[i] != '\0'; i++) {
                if (!isdigit(string[i]))
                    return -1;              /* not a digit char found */
            }
        break;
        case is_float:
            for (i = 0; string[i] != '\0'; i++) {
                if (!isdigit(string[i]) && string[i] != '.')    /* not a digit, nor point */
                    return -1;
            }
        break;
    }

    /* string is ok */
    return 0;
}

/*
 ****************************************************************************
 * Password prompt.
 ****************************************************************************
 */
char * password_prompt(const char *prompt, unsigned int pw_maxlen, bool echo)
{
    struct termios t_orig, t;
    char *password;
    if ((password = (char *) malloc(pw_maxlen + 1)) == NULL) {
        mreport(true, msg_fatal, "FATAL: malloc() for password prompt failed.\n");
    }

    if (!echo) {
        tcgetattr(fileno(stdin), &t);
        t_orig = t;
        t.c_lflag &= ~ECHO;
        tcsetattr(fileno(stdin), TCSAFLUSH, &t);
    }

    if (fputs(prompt, stdout) == EOF) {
        mreport(true, msg_fatal, "FATAL: write to stdout failed.\n");
    }

    if (fgets(password, pw_maxlen + 1, stdin) == NULL)
        password[0] = '\0';

    if (!echo) {
        tcsetattr(fileno(stdin), TCSAFLUSH, &t_orig);
        fputs("\n", stdout);
        fflush(stdout);
    }

    return password;
}

/*
 ****************************************************************************
 * Read input from ncurses window.
 *
 * IN:
 * @window  Window where prompt will be printed.
 * @msg     Message prompt.
 * @pos     At deleting wrong input, cursor do not moving beyond that pos.
 * @len     Max allowed length of string.
 * @echoing Show characters typed by the user.
 *
 * OUT:
 * @with_esc    Flag which determines when function finish with ESC.
 * @str         Entered string.             
 ****************************************************************************
 */
void cmd_readline(WINDOW *window, const char * msg, unsigned int pos, bool * with_esc, char * str, unsigned int len, bool echoing)
{
    int ch;
    int i = 0;
    bool done = false;

    if (echoing)
        echo();
    cbreak();
    nodelay(window, FALSE);
    keypad(window, TRUE);

    /* show prompt if msg not empty */
    if (strlen(msg) != 0) {
        wprintw(window, "%s", msg);
        wrefresh(window);
    }

    memset(str, 0, len);
    while (1) {
        if (done)
            break;
        ch = wgetch(window);
        switch (ch) {
            case ERR:
                str[0] = '\0';
                flushinp();
                done = true;
                break;
            case 27:                            /* Esc */
                wclear(window);
                wprintw(window, "Do nothing. Operation canceled. ");
                nodelay(window, TRUE);
                *with_esc = true;
                str[0] = '\0';
                flushinp();
                done = true;
                break;
            case 10:                            /* Enter */
                str[len] = '\0';
                flushinp();
                nodelay(window, TRUE);
                *with_esc = false;              /* normal finish with \n */
                done = true;
                break;
            case 263: case 330: case 127:       /* Backspace, Delete, */
                if (i > 0) {
                    i--;
                    wdelch(window);
                    continue;
                } else {
                    wmove(window, 0, pos);
                    continue;
                }
                break;
            default:
                if (strlen(str) < len + 1) {
                    str[i] = ch;
                    i++;
                }
                break;
        }
    }

    noecho();
    cbreak();
    nodelay(window, TRUE);
    keypad(window, FALSE);
}
