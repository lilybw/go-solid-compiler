import { template as _$template } from "solid-js/web";
import { style as _$style } from "solid-js/web";
import { className as _$className } from "solid-js/web";
import { setAttribute as _$setAttribute } from "solid-js/web";
import { effect as _$effect } from "solid-js/web";
var _tmpl$ = /*#__PURE__*/_$template(`<div><a title=static>link</a><input disabled><div>`);
export const Attributes = () => (() => {
  var _el$ = _tmpl$(),
    _el$2 = _el$.firstChild,
    _el$3 = _el$2.nextSibling,
    _el$4 = _el$3.nextSibling;
  _$effect(_p$ => {
    var _v$ = url(),
      _v$2 = cls(),
      _v$3 = !!isOn(),
      _v$4 = styles();
    _v$ !== _p$.e && _$setAttribute(_el$2, "href", _p$.e = _v$);
    _v$2 !== _p$.t && _$className(_el$4, _p$.t = _v$2);
    _v$3 !== _p$.a && _el$4.classList.toggle("active", _p$.a = _v$3);
    _p$.o = _$style(_el$4, _v$4, _p$.o);
    return _p$;
  }, {
    e: undefined,
    t: undefined,
    a: undefined,
    o: undefined
  });
  _$effect(() => _el$3.value = val());
  return _el$;
})();
