dnl Macros set with -D and removed with -U; all undefined without them.
ifdef(`cmdline_macro', `cmdline_macro is "cmdline_macro"', `cmdline_macro is not defined')
ifdef(`cmdline_empty', `cmdline_empty is [cmdline_empty]', `cmdline_empty is not defined')
ifdef(`cmdline_gone', `cmdline_gone is still defined', `cmdline_gone is not defined')
ifdef(`cmdline_late', `cmdline_late is "cmdline_late"', `cmdline_late is not defined')
cmdline_macro cmdline_empty cmdline_gone cmdline_late
