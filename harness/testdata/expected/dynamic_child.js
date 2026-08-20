import { template as _$template } from "solid-js/web";
import { insert as _$insert } from "solid-js/web";
var _tmpl$ = /*#__PURE__*/_$template(`<div><h1></h1><p>Hello </p><span>`);
export const DynamicChild = props => (() => {
  var _el$ = _tmpl$(),
    _el$2 = _el$.firstChild,
    _el$3 = _el$2.nextSibling,
    _el$4 = _el$3.firstChild,
    _el$5 = _el$3.nextSibling;
  _$insert(_el$2, () => props.title);
  _$insert(_el$3, () => props.name, null);
  _$insert(_el$5, count);
  return _el$;
})();
