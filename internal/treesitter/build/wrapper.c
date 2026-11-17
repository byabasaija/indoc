// indoc's interface to tree-sitter, compiled once per grammar to WASI.
//
// The host writes source into memory from alloc(), calls parse(), and reads
// a flat pre-order dump of the syntax tree. Each node is seven uint32 words:
//
//   (symbol << 16) | field      grammar symbol id, field id (0 = none)
//   flags                       1 named, 2 extra (comments), 4 error, 8 missing
//   start_byte, end_byte
//   start_row, end_row          0-based
//   child_count                 children follow immediately, in order
//
// Symbol and field names are read once through symbol_name() / field_name().
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include "tree_sitter/api.h"

extern const TSLanguage *LANGUAGE_FN(void);

static TSParser *parser;
static uint32_t *out;
static uint32_t out_len, out_cap;

__attribute__((export_name("alloc"))) void *indoc_alloc(uint32_t n) { return malloc(n); }
__attribute__((export_name("dealloc"))) void indoc_dealloc(void *p) { free(p); }

__attribute__((export_name("symbol_count"))) uint32_t indoc_symbol_count(void) {
  return ts_language_symbol_count(LANGUAGE_FN());
}
__attribute__((export_name("symbol_name"))) const char *indoc_symbol_name(uint32_t id) {
  return ts_language_symbol_name(LANGUAGE_FN(), (TSSymbol)id);
}
__attribute__((export_name("field_count"))) uint32_t indoc_field_count(void) {
  return ts_language_field_count(LANGUAGE_FN());
}
__attribute__((export_name("field_name"))) const char *indoc_field_name(uint32_t id) {
  const char *name = ts_language_field_name_for_id(LANGUAGE_FN(), (TSFieldId)id);
  return name ? name : "";
}

static int push(uint32_t v) {
  if (out_len == out_cap) {
    uint32_t cap = out_cap ? out_cap * 2 : 1u << 16;
    uint32_t *grown = realloc(out, cap * sizeof(uint32_t));
    if (!grown) return 0;
    out = grown;
    out_cap = cap;
  }
  out[out_len++] = v;
  return 1;
}

// parse returns a pointer to the node words; out_words() is their count.
// It returns 0 when memory runs out.
__attribute__((export_name("parse"))) uint32_t *indoc_parse(const char *src, uint32_t len) {
  if (!parser) {
    parser = ts_parser_new();
    ts_parser_set_language(parser, LANGUAGE_FN());
  }
  out_len = 0;
  TSTree *tree = ts_parser_parse_string(parser, NULL, src, len);
  if (!tree) return 0;
  TSTreeCursor cursor = ts_tree_cursor_new(ts_tree_root_node(tree));
  int ok = 1;
  for (;;) {
    TSNode n = ts_tree_cursor_current_node(&cursor);
    uint32_t flags = (ts_node_is_named(n) ? 1 : 0) | (ts_node_is_extra(n) ? 2 : 0) |
                     (ts_node_is_error(n) ? 4 : 0) | (ts_node_is_missing(n) ? 8 : 0);
    TSPoint s = ts_node_start_point(n), e = ts_node_end_point(n);
    ok = ok && push(((uint32_t)ts_node_symbol(n) << 16) | ts_tree_cursor_current_field_id(&cursor));
    ok = ok && push(flags);
    ok = ok && push(ts_node_start_byte(n)) && push(ts_node_end_byte(n));
    ok = ok && push(s.row) && push(e.row);
    ok = ok && push(ts_node_child_count(n));
    if (!ok) break;
    if (ts_tree_cursor_goto_first_child(&cursor)) continue;
    while (!ts_tree_cursor_goto_next_sibling(&cursor)) {
      if (!ts_tree_cursor_goto_parent(&cursor)) goto done;
    }
  }
done:
  ts_tree_cursor_delete(&cursor);
  ts_tree_delete(tree);
  return ok ? out : 0;
}

__attribute__((export_name("out_words"))) uint32_t indoc_out_words(void) { return out_len; }
