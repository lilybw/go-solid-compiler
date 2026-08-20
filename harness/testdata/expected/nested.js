import { template as _$template } from "solid-js/web";
import { insert as _$insert } from "solid-js/web";
var _tmpl$ = /*#__PURE__*/_$template(`<div><section><header><h1></h1></header><main>`);
export const Nested = () => (() => {
  var _el$ = _tmpl$(),
    _el$2 = _el$.firstChild,
    _el$3 = _el$2.firstChild,
    _el$4 = _el$3.firstChild,
    _el$5 = _el$3.nextSibling;
  _$insert(_el$4, title);
  _$insert(_el$5, body);
  return _el$;
})();
