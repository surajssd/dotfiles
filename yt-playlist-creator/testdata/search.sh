#!/usr/bin/env bash
set -euo pipefail

{
    printf 'call\n'
    printf 'arg:%s\n' "$@"
    printf 'credential:%s\n' "${YT_PLAYLIST_CREATOR_OAUTH:-absent}"
} >>"${SEARCH_CALLS}"

case "${!#}" in
*'q=first+song#songs')
    printf 'VIDEO_ID_02\tSearch title, not API metadata\n'
    ;;
*'q=second+song#songs' | *'q=duplicate+song#songs')
    printf 'VIDEO_ID_03\tAnother search title\n'
    ;;
*'q=empty+result#songs') ;;
*'q=failed+result#songs')
    printf 'SECRET from raw process stderr\n' >&2
    exit 17
    ;;
*'q=timeout+result#songs')
    while :; do :; done
    ;;
*'q=bad+id#songs')
    printf 'invalid\tSearch title\n'
    ;;
*'q=two+results#songs')
    printf 'VIDEO_ID_02\tOne\nVIDEO_ID_03\tTwo\n'
    ;;
*'q=missing+title#songs')
    printf 'VIDEO_ID_02\t\n'
    ;;
*'q=missing+tab#songs')
    printf 'VIDEO_ID_02 A title\n'
    ;;
*'q=large+result#songs')
    printf '%*s' 70000 ''
    ;;
*'q=invalid+utf8#songs')
    printf 'VIDEO_ID_02\t\377\n'
    ;;
*)
    printf 'VIDEO_ID_04\tSearch title\r\n'
    ;;
esac
