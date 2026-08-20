import { template as _$template } from "solid-js/web";
import { spread as _$spread } from "solid-js/web";
import { mergeProps as _$mergeProps } from "solid-js/web";
var _tmpl$ = /*#__PURE__*/_$template(`<div><input>`);
export const HostSpread = props => (() => {
  var _el$ = _tmpl$(),
    _el$2 = _el$.firstChild;
  _$spread(_el$, _$mergeProps(props, {
    "id": "fixed"
  }), false, true);
  _$spread(_el$2, _$mergeProps(inputProps), false, false);
  return _el$;
})();
