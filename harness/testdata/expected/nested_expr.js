import { template as _$template } from "solid-js/web";
import { className as _$className } from "solid-js/web";
import { effect as _$effect } from "solid-js/web";
import { insert as _$insert } from "solid-js/web";
var _tmpl$ = /*#__PURE__*/_$template(`<ul>`),
  _tmpl$2 = /*#__PURE__*/_$template(`<li>`);
export const NestedExpr = () => (() => {
  var _el$ = _tmpl$();
  _$insert(_el$, () => items().map(i => (() => {
    var _el$2 = _tmpl$2();
    _$insert(_el$2, () => i.label);
    _$effect(() => _$className(_el$2, i.cls));
    return _el$2;
  })()));
  return _el$;
})();
