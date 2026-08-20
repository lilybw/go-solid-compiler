import { template as _$template } from "solid-js/web";
import { insert as _$insert } from "solid-js/web";
import { createComponent as _$createComponent } from "solid-js/web";
var _tmpl$ = /*#__PURE__*/_$template(`<span>child`),
  _tmpl$2 = /*#__PURE__*/_$template(`<div>`);
export const Components = () => (() => {
  var _el$ = _tmpl$2();
  _$insert(_el$, _$createComponent(Card, {
    title: "static",
    get count() {
      return count();
    }
  }), null);
  _$insert(_el$, _$createComponent(Wrapper, {
    get children() {
      return _tmpl$();
    }
  }), null);
  _$insert(_el$, _$createComponent(UI.Button, {
    onClick: go,
    children: "Go"
  }), null);
  return _el$;
})();
