import { template as _$template } from "solid-js/web";
import { setStyleProperty as _$setStyleProperty } from "solid-js/web";
import { setAttribute as _$setAttribute } from "solid-js/web";
import { effect as _$effect } from "solid-js/web";
import { use as _$use } from "solid-js/web";
import { addEventListener as _$addEventListener } from "solid-js/web";
var _tmpl$ = /*#__PURE__*/_$template(`<div>`);
export const Namespaces = () => (() => {
  var _el$ = _tmpl$();
  var _ref$ = myRef;
  typeof _ref$ === "function" ? _$use(_ref$, _el$) : myRef = _el$;
  _$addEventListener(_el$, "CustomEvent", handler);
  _$effect(_p$ => {
    var _v$ = v(),
      _v$2 = id(),
      _v$3 = color(),
      _v$4 = !!isOn();
    _v$ !== _p$.e && (_el$.value = _p$.e = _v$);
    _v$2 !== _p$.t && _$setAttribute(_el$, "data-id", _p$.t = _v$2);
    _v$3 !== _p$.a && _$setStyleProperty(_el$, "color", _p$.a = _v$3);
    _v$4 !== _p$.o && _el$.classList.toggle("active", _p$.o = _v$4);
    return _p$;
  }, {
    e: undefined,
    t: undefined,
    a: undefined,
    o: undefined
  });
  return _el$;
})();
