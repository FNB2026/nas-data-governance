char *ndg_access_pick(void **scope);
char *ndg_access_resolve(const char *bookmark, const char *expectedRoot, int expectedNetwork, void **scope);
void ndg_access_release(void *scope);
void ndg_access_cancel_picker(void);
char *ndg_access_support(void);
