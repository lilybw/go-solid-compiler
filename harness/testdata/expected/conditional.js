import { template as _$template } from "solid-js/web";
import { insert as _$insert } from "solid-js/web";
import { memo as _$memo } from "solid-js/web";
var _tmpl$ = /*#__PURE__*/_$template(`<div>`),
  _tmpl$2 = /*#__PURE__*/_$template(`<a href=/y>yes`),
  _tmpl$3 = /*#__PURE__*/_$template(`<b>no`),
  _tmpl$4 = /*#__PURE__*/_$template(`<span>maybe`);
export const Conditional = () => (() => {
  var _el$ = _tmpl$();
  _$insert(_el$, (() => {
    var _c$ = _$memo(() => !!cond());
    return () => _c$() ? _tmpl$2() : _tmpl$3();
  })(), null);
  _$insert(_el$, (() => {
    var _c$2 = _$memo(() => !!flag());
    return () => _c$2() && _tmpl$4();
  })(), null);
  return _el$;
})();
