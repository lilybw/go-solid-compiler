import { template as _$template } from "solid-js/web";
import { delegateEvents as _$delegateEvents } from "solid-js/web";
import { addEventListener as _$addEventListener } from "solid-js/web";
var _tmpl$ = /*#__PURE__*/_$template(`<div><button>+</button><input><div>`);
export const Events = () => (() => {
  var _el$ = _tmpl$(),
    _el$2 = _el$.firstChild,
    _el$3 = _el$2.nextSibling,
    _el$4 = _el$3.nextSibling;
  _$addEventListener(_el$2, "click", inc, true);
  _$addEventListener(_el$3, "input", onInput, true);
  _$addEventListener(_el$4, "scroll", onScroll);
  return _el$;
})();
_$delegateEvents(["click", "input"]);
